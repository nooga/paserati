package parser

import (
	"reflect"
	"strings"

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
	p.reportUnresolvedPrivateName(tok)
}

func (p *Parser) reportUnresolvedPrivateName(tok *lexer.Token) {
	if p.outerPrivateNames[tok.Literal] {
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
	type privateDecl struct {
		kind     string // "field", "method", "getter", "setter"
		isStatic bool
	}
	declared := map[string][]privateDecl{}
	declare := func(tok *lexer.Token, name, kind string, isStatic bool) {
		if name == "#constructor" {
			p.addError(tok, "Classes may not have a private field named '#constructor'")
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
		if dup {
			p.addError(tok, "Identifier '"+name+"' has already been declared")
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
				p.addError(m.Token, "Class constructor may not be an accessor, generator or async method")
			} else if m.Value != nil && m.Value.Body != nil {
				constructors++
				if constructors > 1 {
					p.addError(m.Token, "A class may only have one constructor")
				}
			}
		case m.IsStatic && name == "prototype":
			p.addError(m.Token, "Classes may not have a static property named 'prototype'")
		}
	}

	for _, block := range body.StaticInitializers {
		p.checkInitializerEarlyErrors(block)
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
				p.addError(prop.Token, "Classes may not have a field named 'constructor'")
			} else if prop.IsStatic && name == "prototype" {
				p.addError(prop.Token, "Classes may not have a static property named 'prototype'")
			}
		}
	}

	for _, ref := range scope.refs {
		if _, ok := declared[ref.Literal]; ok {
			continue
		}
		p.notePrivateReference(ref)
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
