package parser

import (
	"reflect"
	"sort"
	"strings"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
)

// privateNameScope collects the private names referenced while parsing one
// class body. References are resolved when the body ends: names the class
// declares are satisfied, the rest move to the enclosing class, and at the
// outermost class any leftover is an early SyntaxError
// (ECMA-262 15.7.1 AllPrivateIdentifiersValid).
type privateNameScope struct {
	refs []*lexer.Token
}

// SetOuterPrivateNames supplies the private names visible at a direct eval
// call site (the PrivateEnvironment of the calling code). References to them
// from the eval source are valid even though no class in that source
// declares them. Names are given without the leading '#'.
func (p *Parser) SetOuterPrivateNames(names []string) {
	p.outerPrivateNames = make(map[string]bool, len(names))
	for _, n := range names {
		p.outerPrivateNames["#"+n] = true
	}
}

// notePrivateReference records a use of a private name (obj.#x, obj?.#x,
// #x in obj) for AllPrivateIdentifiersValid.
func (p *Parser) notePrivateReference(tok *lexer.Token) {
	if n := len(p.privateScopes); n > 0 {
		p.privateScopes[n-1].refs = append(p.privateScopes[n-1].refs, tok)
		return
	}
	p.reportUnresolvedPrivateName(tok, false)
}

// bubblePrivateReference passes a reference no class declared to the
// enclosing class scope, or reports it.
func (p *Parser) bubblePrivateReference(tok *lexer.Token) {
	if n := len(p.privateScopes); n > 0 {
		p.privateScopes[n-1].refs = append(p.privateScopes[n-1].refs, tok)
		return
	}
	p.reportUnresolvedPrivateName(tok, true)
}

func (p *Parser) reportUnresolvedPrivateName(tok *lexer.Token, inClass bool) {
	if p.outerPrivateNames[tok.Literal] {
		return
	}
	if p.tscDiagnostics {
		// tsc leaves an undeclared private name to the checker, which needs
		// to know whether the reference sits in a class body at all.
		if p.unresolvedPrivateNames == nil {
			p.unresolvedPrivateNames = map[*lexer.Token]bool{}
		}
		p.unresolvedPrivateNames[tok] = inClass
		return
	}
	p.addError(tok, "Private field '"+tok.Literal+"' must be declared in an enclosing class")
}

// strictReservedClassNames are the identifiers a class may not be named:
// all of a class is strict mode code, so its BindingIdentifier is subject
// to the strict mode restrictions (ECMA-262 13.1.1).
var strictReservedClassNames = map[string]bool{
	"let": true, "static": true, "yield": true, "implements": true, "interface": true,
	"package": true, "private": true, "protected": true, "public": true,
	"eval": true, "arguments": true,
}

func (p *Parser) checkClassName(name *Identifier) {
	if name != nil && strictReservedClassNames[name.Value] {
		p.addError(name.Token, "Unexpected strict mode reserved word '"+name.Value+"'")
	}
}

// checkClassHeritage rejects a ClassHeritage that is not a
// LeftHandSideExpression. The parser reads the heritage with the general
// expression grammar, so an unparenthesized arrow function gets through.
func (p *Parser) checkClassHeritage(superClass Expression) {
	if arrow, ok := superClass.(*ArrowFunctionLiteral); ok && !arrow.Parenthesized {
		p.addError(arrow.Token, "Class heritage must be a left-hand-side expression")
	}
}

// privateKeyName returns the name of a private class element key.
func privateKeyName(key Expression) (string, bool) {
	if id, ok := key.(*Identifier); ok && strings.HasPrefix(id.Value, "#") {
		return id.Value, true
	}
	return "", false
}

// staticKeyName returns the PropName of a non-computed class element key.
func staticKeyName(key Expression) (string, bool) {
	switch k := key.(type) {
	case *Identifier:
		if strings.HasPrefix(k.Value, "#") {
			return "", false
		}
		return k.Value, true
	case *StringLiteral:
		return k.Value, true
	}
	return "", false
}

// checkClassBodyEarlyErrors applies the ClassBody and ClassElement static
// semantics rules (ECMA-262 15.7.1) that need the whole body, and resolves
// the private-name references collected in scope.
func (p *Parser) checkClassBodyEarlyErrors(body *ClassBody, scope *privateNameScope) {
	if p.tscDiagnostics && body != nil {
		p.checkClassDecoratorPlacement(body)
	}
	type privateDecl struct {
		kind     string // "field", "method", "getter", "setter"
		isStatic bool
	}
	className := p.classBodyName
	staticPrototype := func(tok *lexer.Token) {
		p.addSemanticErrorWithCode(tok, "TS2699", "Static property 'prototype' conflicts with built-in property 'Function.prototype' of constructor function '"+className+"'.")
	}
	declared := map[string][]privateDecl{}
	declare := func(tok *lexer.Token, name, kind string, isStatic bool) {
		if name == "#constructor" {
			p.addSemanticErrorWithCode(tok, "TS18012", "'#constructor' is a reserved word.")
			return
		}
		prev := declared[name]
		dup := len(prev) > 0
		// A private getter and setter may share a name when both are static
		// or both are non-static; any other repetition is a duplicate.
		if len(prev) == 1 && prev[0].isStatic == isStatic &&
			((prev[0].kind == "getter" && kind == "setter") || (prev[0].kind == "setter" && kind == "getter")) {
			dup = false
		}
		// tsc reports duplicate private names from its binder and checker
		// (see bindClassBody in binder_dups.go), not as a syntax error.
		if dup && !p.tscDiagnostics {
			before := len(p.errors)
			p.addError(tok, "Identifier '"+name+"' has already been declared")
			if len(p.errors) > before {
				if se, ok := p.errors[len(p.errors)-1].(*errors.SyntaxError); ok {
					p.redeclarationErrors = append(p.redeclarationErrors, se)
				}
			}
		}
		declared[name] = append(prev, privateDecl{kind, isStatic})
	}

	constructors := 0
	for _, m := range body.Methods {
		if name, ok := privateKeyName(m.Key); ok {
			kind := "method"
			if m.Kind == "getter" || m.Kind == "setter" {
				kind = m.Kind
			}
			declare(m.Token, name, kind, m.IsStatic)
			continue
		}
		name, ok := staticKeyName(m.Key)
		if !ok {
			continue
		}
		special := m.Kind == "getter" || m.Kind == "setter" ||
			(m.Value != nil && (m.Value.IsGenerator || m.Value.IsAsync))
		switch {
		case !m.IsStatic && name == "constructor":
			if special {
				switch {
				case m.Kind == "getter" || m.Kind == "setter":
					p.addErrorWithCode(m.Token, "TS1341", "Class constructor may not be an accessor.")
				case m.Value != nil && m.Value.IsGenerator:
					p.addErrorWithCode(m.Token, "TS1368", "Class constructor may not be a generator.")
				default:
					p.addErrorWithCode(m.Token, "TS1089", "'async' modifier cannot appear on a constructor declaration.")
				}
			} else if m.Value != nil && m.Value.Body != nil {
				constructors++
				if constructors > 1 {
					p.addError(m.Token, "A class may only have one constructor")
				}
			}
		case m.IsStatic && name == "prototype":
			staticPrototype(m.Token)
		}
	}

	for _, block := range body.StaticInitializers {
		p.checkInitializerEarlyErrors(block)
	}

	if p.tscDiagnostics && p.experimentalDecorators {
		p.checkLegacyAccessorDecorators(body)
	}

	for _, prop := range body.Properties {
		if prop.Value != nil {
			p.checkInitializerEarlyErrors(prop.Value)
		}
		if name, ok := privateKeyName(prop.Key); ok {
			declare(prop.Token, name, "field", prop.IsStatic)
			continue
		}
		if name, ok := staticKeyName(prop.Key); ok {
			if name == "constructor" {
				p.addGrammarErrorWithCode(prop.Token, "TS18006", "Classes may not have a field named 'constructor'.")
			} else if prop.IsStatic && name == "prototype" {
				staticPrototype(prop.Token)
			}
		}
	}

	for _, ref := range scope.refs {
		if _, ok := declared[ref.Literal]; ok {
			continue
		}
		p.bubblePrivateReference(ref)
	}
}

// isPrivateReference reports whether expr (the operand of delete) is a
// property reference whose final step is a private name: x.#y, x?.#y,
// x?.y.#z and so on. Parentheses are transparent here, matching the
// CoverParenthesizedExpression rule for delete (ECMA-262 13.5.1.1).
func isPrivateReference(expr Expression) bool {
	switch e := expr.(type) {
	case *MemberExpression:
		_, ok := privateKeyName(e.Property)
		return ok
	case *OptionalChainingExpression:
		if e.Continuation != nil {
			return isPrivateReference(e.Continuation)
		}
		_, ok := privateKeyName(e.Property)
		return ok
	case *OptionalIndexExpression:
		return e.Continuation != nil && isPrivateReference(e.Continuation)
	case *OptionalCallExpression:
		return e.Continuation != nil && isPrivateReference(e.Continuation)
	}
	return false
}

// checkClassFieldEnd enforces that a field definition is terminated by ';',
// the closing '}', or a line break (ASI): `x y` or `#x #y` on one line is a
// SyntaxError. last is the field's final token and next the one after it.
func (p *Parser) checkClassFieldEnd(last, next *lexer.Token) {
	if last == nil || next == nil {
		return
	}
	switch next.Type {
	case lexer.SEMICOLON, lexer.RBRACE, lexer.EOF:
		return
	}
	if next.Line == last.Line {
		// tsc parseSemicolonAfterPropertyName: a decorator after the name
		// gets its own message; the decorator then starts the next member.
		if p.tscDiagnostics && next.Type == lexer.AT {
			p.addErrorWithCode(next, "TS1436", "Decorators must precede the name and all keywords of property declarations.")
			return
		}
		p.addError(next, "';' expected.")
	}
}

// initializerHazard is what scanInitializer found in a field initializer
// or static block.
type initializerHazard struct {
	arguments *Identifier     // an `arguments` IdentifierReference
	superCall *CallExpression // a super(...) call
}

// checkInitializerEarlyErrors applies the FieldDefinition and
// ClassStaticBlock rules: an initializer or static block may not contain an
// `arguments` reference (ContainsArguments) or a SuperCall. Arrow functions
// are looked through; other functions and methods have their own
// `arguments` and are not.
func (p *Parser) checkInitializerEarlyErrors(node Node) {
	if node == nil || reflect.ValueOf(node).IsNil() {
		return
	}
	var h initializerHazard
	scanInitializer(reflect.ValueOf(node), &h)
	if p.tscDiagnostics {
		if h.arguments != nil {
			p.addErrorWithCode(h.arguments.Token, "TS2815", "'arguments' cannot be referenced in property initializers or class static initialization blocks.")
		}
		if h.superCall != nil {
			tok := h.superCall.Token
			if sup, ok := h.superCall.Function.(*SuperExpression); ok && sup.Token != nil {
				tok = sup.Token
			}
			p.addErrorWithCode(tok, "TS2337", "Super calls are not permitted outside constructors or in nested functions inside constructors.")
		}
		return
	}
	if h.arguments != nil {
		p.addError(h.arguments.Token, "'arguments' is not allowed in class field initializer or static initialization block")
	}
	if h.superCall != nil {
		p.addError(h.superCall.Token, "'super' keyword unexpected here")
	}
}

var nodeType = reflect.TypeOf((*Node)(nil)).Elem()

// scanInitializer walks an AST subtree generically, special-casing the node
// types where ContainsArguments / Contains SuperCall do not simply recurse.
func scanInitializer(v reflect.Value, h *initializerHazard) {
	if h.arguments != nil && h.superCall != nil {
		return
	}
	for v.Kind() == reflect.Interface {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return
	}
	switch n := v.Interface().(type) {
	case *Identifier:
		if n.Value == "arguments" && h.arguments == nil {
			h.arguments = n
		}
		return
	case *FunctionLiteral:
		return
	case *CallExpression:
		if _, ok := n.Function.(*SuperExpression); ok && h.superCall == nil {
			h.superCall = n
		}
	case *MemberExpression:
		scanInitializer(reflect.ValueOf(n.Object), h)
		return
	case *OptionalChainingExpression:
		scanInitializer(reflect.ValueOf(n.Object), h)
		scanInitializer(reflect.ValueOf(n.Continuation), h)
		return
	case *ObjectProperty:
		if _, plain := n.Key.(*Identifier); !plain {
			scanInitializer(reflect.ValueOf(n.Key), h)
		}
		scanInitializer(reflect.ValueOf(n.Value), h)
		return
	case *ClassExpression:
		scanInitializer(reflect.ValueOf(n.SuperClass), h)
		scanClassComputedKeys(n.Body, h)
		return
	case *ClassDeclaration:
		scanInitializer(reflect.ValueOf(n.SuperClass), h)
		scanClassComputedKeys(n.Body, h)
		return
	}
	s := v.Elem()
	if s.Kind() != reflect.Struct {
		return
	}
	t := s.Type()
	for i := 0; i < s.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() || f.Anonymous || strings.Contains(f.Name, "Type") {
			continue // skip BaseExpression, type annotations and type arguments
		}
		fv := s.Field(i)
		switch fv.Kind() {
		case reflect.Interface, reflect.Ptr:
			if fv.Kind() == reflect.Interface || fv.Type().Implements(nodeType) {
				scanInitializer(fv, h)
			}
		case reflect.Slice:
			if et := fv.Type().Elem(); et.Kind() == reflect.Interface || et.Implements(nodeType) {
				for j := 0; j < fv.Len(); j++ {
					scanInitializer(fv.Index(j), h)
				}
			}
		}
	}
}

// scanClassComputedKeys visits the only parts of a nested class body that
// belong to the enclosing initializer: computed element names.
func scanClassComputedKeys(body *ClassBody, h *initializerHazard) {
	if body == nil {
		return
	}
	for _, m := range body.Methods {
		if c, ok := m.Key.(*ComputedPropertyName); ok {
			scanInitializer(reflect.ValueOf(c.Expr), h)
		}
	}
	for _, prop := range body.Properties {
		if c, ok := prop.Key.(*ComputedPropertyName); ok {
			scanInitializer(reflect.ValueOf(c.Expr), h)
		}
	}
}

// checkClassDecoratorPlacement reports TS1206 for decorators tsc does not
// allow: on a constructor, on a #private member under
// --experimentalDecorators, and on parameters without it.
func (p *Parser) checkClassDecoratorPlacement(body *ClassBody) {
	bad := func(ds []*Decorator) {
		if len(ds) > 0 && ds[0] != nil && ds[0].Token != nil {
			p.addErrorWithCode(ds[0].Token, errors.TS1206, "Decorators are not valid here.")
		}
	}
	for _, m := range body.Methods {
		if m == nil {
			continue
		}
		_, private := privateKeyName(m.Key)
		if m.Kind == "constructor" || (private && p.experimentalDecorators) {
			bad(m.Decorators)
		}
		if !p.experimentalDecorators && m.Value != nil {
			for _, prm := range m.Value.Parameters {
				if prm != nil {
					bad(prm.Decorators)
				}
			}
		}
	}
	if p.experimentalDecorators {
		for _, prop := range body.Properties {
			if prop == nil {
				continue
			}
			if _, private := privateKeyName(prop.Key); private {
				bad(prop.Decorators)
			}
		}
	}
}

// checkLegacyAccessorDecorators is checkGrammarModifiers' legacy-decorator
// rule: of a get/set pair only the first accessor may be decorated (TS1207
// on the second when both are).
func (p *Parser) checkLegacyAccessorDecorators(body *ClassBody) {
	type key struct {
		name     string
		isStatic bool
	}
	first := map[key]*MethodDefinition{}
	var accessors []*MethodDefinition
	for _, m := range body.Methods {
		if m != nil && (m.Kind == "getter" || m.Kind == "setter") {
			accessors = append(accessors, m)
		}
	}
	sort.SliceStable(accessors, func(i, j int) bool { return accessors[i].Token.StartPos < accessors[j].Token.StartPos })
	for _, m := range accessors {
		name, _, _ := propertyKeyName(m.Key)
		if name == "" {
			continue
		}
		k := key{name, m.IsStatic}
		f, ok := first[k]
		if !ok {
			first[k] = m
			continue
		}
		if f.Kind != m.Kind && len(f.Decorators) > 0 && len(m.Decorators) > 0 && m.Decorators[0] != nil {
			p.addGrammarErrorWithCode(m.Decorators[0].Token, "TS1207", "Decorators cannot be applied to multiple get/set accessors of the same name.")
		}
	}
}
