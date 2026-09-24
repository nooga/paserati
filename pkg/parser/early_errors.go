package parser

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
)

// Identifier early errors.
//
// The Pratt parser decides whether `yield`, `await`, `let` and friends are
// keywords with local heuristics, and strictness only becomes known once a
// function body's directive prologue has been read - after its parameters and
// name were already parsed. So instead of threading every ECMAScript grammar
// parameter ([Yield], [Await], strict) through each parse function, the
// identifier-related static semantics are checked here in one pass over the
// finished AST, which knows every enclosing function's kind and strictness:
//
//   - reserved words (including ones spelled with \u escapes) used as an
//     IdentifierReference, BindingIdentifier or LabelIdentifier;
//   - strict-mode reserved words, and `eval`/`arguments` as binding names or
//     assignment targets in strict code;
//   - `yield` in generators and strict code, `await` in async functions and
//     class static blocks;
//   - YieldExpression/AwaitExpression in formal parameters and AwaitExpression
//     in class static blocks;
//   - "use strict" in a function with a non-simple parameter list, and
//     duplicate parameter names where they are forbidden.
//
// Nodes the pass does not know are skipped, so an unhandled construct can only
// miss an error, never invent one.

var reservedWords = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true,
	"continue": true, "debugger": true, "default": true, "delete": true, "do": true,
	"else": true, "enum": true, "export": true, "extends": true, "false": true,
	"finally": true, "for": true, "function": true, "if": true, "import": true,
	"in": true, "instanceof": true, "new": true, "null": true, "return": true,
	"super": true, "switch": true, "this": true, "throw": true, "true": true,
	"try": true, "typeof": true, "var": true, "void": true, "while": true, "with": true,
}

var strictReservedWords = map[string]bool{
	"implements": true, "interface": true, "let": true, "package": true,
	"private": true, "protected": true, "public": true, "static": true, "yield": true,
}

type eeCtx struct {
	strict      bool
	inClass     bool // strict because inside a class (for the TypeScript diagnostic wording)
	yield       bool // [+Yield]: `yield` is a keyword here
	await       bool // [+Await]: `await` is a keyword here
	staticBlock bool // directly inside a class static block (not a nested function/arrow body)
	noYieldExpr bool // formal parameters where a YieldExpression is an error
	noAwaitExpr bool // formal parameters where an AwaitExpression is an error
}

type eeWalker struct {
	p *Parser
	// collect, when non-nil, receives every BindingIdentifier visited by
	// binding() - used to find duplicate parameter names.
	collect *[]*Identifier
}

// checkEarlyErrors runs the identifier early-error pass over a parsed program.
func (p *Parser) checkEarlyErrors(program *Program, initialStrict bool) {
	w := &eeWalker{p: p}
	c := eeCtx{strict: initialStrict}
	if tok := useStrictDirective(program.Statements); tok != nil {
		c.strict = true
	}
	w.stmts(program.Statements, c)
}

func (w *eeWalker) errorf(tok *lexer.Token, format string, args ...interface{}) {
	if tok == nil {
		return
	}
	w.p.addError(tok, fmt.Sprintf(format, args...))
}

// useStrictDirective returns the token of a "use strict" directive in the
// directive prologue of stmts, or nil.
func useStrictDirective(stmts []Statement) *lexer.Token {
	for _, s := range stmts {
		es, ok := s.(*ExpressionStatement)
		if !ok {
			return nil
		}
		str, ok := es.Expression.(*StringLiteral)
		if !ok {
			return nil
		}
		if str.Value == "use strict" && !str.HasEscape {
			return str.Token
		}
	}
	return nil
}

// ident applies the rules shared by IdentifierReference, BindingIdentifier and
// LabelIdentifier.
func (w *eeWalker) ident(id *Identifier, c eeCtx) {
	if id == nil || id.Token == nil {
		return
	}
	name := id.Value
	switch {
	case reservedWords[name]:
		w.errorf(id.Token, "Unexpected reserved word '%s'", name)
	case name == "yield" && (c.yield || c.strict):
		if c.strict && !c.yield {
			w.strictReserved(id, c)
		} else {
			w.errorf(id.Token, "'yield' is not a valid identifier in a generator")
		}
	case c.strict && strictReservedWords[name]:
		w.strictReserved(id, c)
	case name == "await" && (c.await || c.staticBlock):
		w.errorf(id.Token, "'await' is not a valid identifier here")
	}
}

// strictReserved reports a strict-mode future reserved word used as an
// identifier, worded like TypeScript's TS1212/TS1213.
func (w *eeWalker) strictReserved(id *Identifier, c eeCtx) {
	code, why := errors.TS1212, ""
	if c.inClass {
		code, why = errors.TS1213, " Class definitions are automatically in strict mode."
	}
	w.p.addErrorWithCode(id.Token, code, "Identifier expected. '"+id.Value+"' is a reserved word in strict mode."+why)
}

func (w *eeWalker) binding(id *Identifier, c eeCtx) {
	if id == nil {
		return
	}
	if w.collect != nil {
		*w.collect = append(*w.collect, id)
	}
	w.ident(id, c)
	if c.strict && (id.Value == "eval" || id.Value == "arguments") {
		w.evalOrArguments(id)
	}
}

func (w *eeWalker) evalOrArguments(id *Identifier) {
	w.p.addErrorWithCode(id.Token, errors.TS1100, "Invalid use of '"+id.Value+"' in strict mode.")
}

// simpleTarget checks an Identifier used as a simple assignment target.
func (w *eeWalker) simpleTarget(id *Identifier, c eeCtx) {
	w.ident(id, c)
	if c.strict && (id.Value == "eval" || id.Value == "arguments") {
		w.evalOrArguments(id)
	}
}

// --- binding patterns ---

func (w *eeWalker) pattern(e Expression, c eeCtx) {
	switch t := e.(type) {
	case nil:
	case *Identifier:
		w.binding(t, c)
	case *ArrayParameterPattern:
		w.destructElems(t.Elements, c, true)
	case *ObjectParameterPattern:
		w.destructProps(t.Properties, t.RestProperty, c, true)
	case *AssignmentExpression:
		w.pattern(t.Left, c)
		w.expr(t.Value, c)
	case *SpreadElement:
		w.pattern(t.Argument, c)
	case *ArrayLiteral:
		for _, el := range t.Elements {
			w.pattern(el, c)
		}
	case *ObjectLiteral:
		for _, prop := range t.Properties {
			w.patternProp(prop, c, true)
		}
	default:
		// Not a binding pattern shape; still visit for nested expressions.
		w.expr(e, c)
	}
}

func (w *eeWalker) patternProp(prop *ObjectProperty, c eeCtx, binding bool) {
	if prop == nil {
		return
	}
	target := prop.Value
	if target == nil || isShorthand(prop) {
		// `{a}` / `{a = 1}` / `{...r}`: the target lives in Key (or Value
		// is the same identifier).
		if target == nil {
			target = prop.Key
		}
	} else {
		w.propKey(prop.Key, c)
	}
	if binding {
		w.pattern(target, c)
	} else {
		w.target(target, c)
	}
}

func (w *eeWalker) destructElems(elems []*DestructuringElement, c eeCtx, binding bool) {
	for _, el := range elems {
		if el == nil {
			continue
		}
		if binding {
			w.pattern(el.Target, c)
		} else {
			w.target(el.Target, c)
		}
		w.expr(el.Default, c)
	}
}

func (w *eeWalker) destructProps(props []*DestructuringProperty, rest *DestructuringElement, c eeCtx, binding bool) {
	for _, prop := range props {
		if prop == nil {
			continue
		}
		if prop.Key != prop.Target {
			w.propKey(prop.Key, c)
		}
		if binding {
			w.pattern(prop.Target, c)
		} else {
			w.target(prop.Target, c)
		}
		w.expr(prop.Default, c)
	}
	if rest != nil {
		if binding {
			w.pattern(rest.Target, c)
		} else {
			w.target(rest.Target, c)
		}
	}
}

// --- assignment targets ---

func (w *eeWalker) target(e Expression, c eeCtx) {
	switch t := e.(type) {
	case nil:
	case *Identifier:
		w.simpleTarget(t, c)
	case *AssignmentExpression:
		w.target(t.Left, c)
		w.expr(t.Value, c)
	case *SpreadElement:
		w.target(t.Argument, c)
	case *ArrayLiteral:
		for i, el := range t.Elements {
			if _, ok := el.(*SpreadElement); ok && (i != len(t.Elements)-1 || t.CommaAfterSpread) {
				w.errorf(t.Token, "Rest element must be last element")
			}
			w.target(el, c)
		}
	case *ObjectLiteral:
		for i, prop := range t.Properties {
			if prop != nil && i != len(t.Properties)-1 {
				if _, ok := prop.Key.(*SpreadElement); ok {
					w.errorf(t.Token, "Rest element must be last element")
				}
			}
			w.patternProp(prop, c, false)
		}
	case *ArrayDestructuringAssignment:
		w.destructElems(t.Elements, c, false)
		w.expr(t.Value, c)
	case *ObjectDestructuringAssignment:
		w.destructProps(t.Properties, t.RestProperty, c, false)
		w.expr(t.Value, c)
	default:
		w.expr(e, c)
	}
}

// isShorthand reports whether an object literal property was written as
// `{a}` (or a cover-grammar `{a = 1}`), where the value is an
// IdentifierReference rather than a property name.
func isShorthand(prop *ObjectProperty) bool {
	key, ok := prop.Key.(*Identifier)
	if !ok {
		return false
	}
	switch v := prop.Value.(type) {
	case *Identifier:
		return v == key || (v.Token != nil && v.Token == key.Token)
	case *AssignmentExpression:
		if l, ok := v.Left.(*Identifier); ok {
			return l == key || (l.Token != nil && l.Token == key.Token)
		}
	}
	return false
}

func (w *eeWalker) propKey(key Expression, c eeCtx) {
	switch k := key.(type) {
	case *ComputedPropertyName:
		w.expr(k.Expr, c)
	case *SpreadElement:
		w.expr(k.Argument, c)
	}
}

// --- statements ---

func (w *eeWalker) stmts(list []Statement, c eeCtx) {
	for _, s := range list {
		w.stmt(s, c)
	}
}

func (w *eeWalker) block(b *BlockStatement, c eeCtx) {
	if b != nil {
		w.stmts(b.Statements, c)
	}
}

func (w *eeWalker) declarators(decls []*VarDeclarator, name *Identifier, value Expression, c eeCtx) {
	if len(decls) == 0 {
		decls = []*VarDeclarator{{Name: name, Value: value}}
	}
	for _, d := range decls {
		if d == nil {
			continue
		}
		if d.Name != nil {
			w.binding(d.Name, c)
		}
		w.expr(d.Value, c)
	}
}

func (w *eeWalker) stmt(s Statement, c eeCtx) {
	switch t := s.(type) {
	case nil:
	case *ExpressionStatement:
		if fn, ok := t.Expression.(*FunctionLiteral); ok && fn.Name != nil && !fn.Parenthesized {
			w.function(fn, c, true, false)
			return
		}
		w.expr(t.Expression, c)
	case *VarStatement:
		w.declarators(t.Declarations, t.Name, t.Value, c)
	case *LetStatement:
		w.declarators(t.Declarations, t.Name, t.Value, c)
	case *ConstStatement:
		w.declarators(t.Declarations, t.Name, t.Value, c)
	case *ArrayDestructuringDeclaration:
		w.destructElems(t.Elements, c, true)
		w.expr(t.Value, c)
	case *ObjectDestructuringDeclaration:
		w.destructProps(t.Properties, t.RestProperty, c, true)
		w.expr(t.Value, c)
	case *DeclarationGroup:
		w.stmts(t.Declarations, c)
	case *ReturnStatement:
		w.expr(t.ReturnValue, c)
	case *ThrowStatement:
		w.expr(t.Value, c)
	case *BlockStatement:
		w.block(t, c)
	case *IfStatement:
		w.expr(t.Condition, c)
		w.block(t.Consequence, c)
		w.block(t.Alternative, c)
	case *WhileStatement:
		w.expr(t.Condition, c)
		w.block(t.Body, c)
	case *DoWhileStatement:
		w.block(t.Body, c)
		w.expr(t.Condition, c)
	case *ForStatement:
		w.forHead(t.Initializer, c)
		w.expr(t.Condition, c)
		w.expr(t.Update, c)
		w.block(t.Body, c)
	case *ForInStatement:
		w.forHead(t.Variable, c)
		w.expr(t.Object, c)
		w.block(t.Body, c)
	case *ForOfStatement:
		w.forHead(t.Variable, c)
		w.expr(t.Iterable, c)
		w.block(t.Body, c)
	case *LabeledStatement:
		w.ident(t.Label, c)
		w.stmt(t.Statement, c)
	case *BreakStatement:
		w.ident(t.Label, c)
	case *ContinueStatement:
		w.ident(t.Label, c)
	case *WithStatement:
		w.expr(t.Expression, c)
		w.stmt(t.Body, c)
	case *TryStatement:
		w.block(t.Body, c)
		if t.CatchClause != nil {
			w.pattern(t.CatchClause.Parameter, c)
			w.block(t.CatchClause.Body, c)
		}
		w.block(t.FinallyBlock, c)
	case *SwitchStatement:
		w.expr(t.Expression, c)
		for _, cs := range t.Cases {
			if cs != nil {
				w.expr(cs.Condition, c)
				w.block(cs.Body, c)
			}
		}
	case *ClassDeclaration:
		// The class binding is resolved in the enclosing context, but class
		// code (and so the binding name check for reserved words) is strict.
		if t.Name != nil {
			nc := c
			nc.strict = true
			w.binding(t.Name, nc)
		}
		w.class(t.SuperClass, t.Body, c)
	case *FunctionOverloadGroup:
		if t.Implementation != nil {
			w.function(t.Implementation, c, true, false)
		}
	case *ExportNamedDeclaration:
		w.stmt(t.Declaration, c)
	case *ExportDefaultDeclaration:
		if fn, ok := t.Declaration.(*FunctionLiteral); ok && fn.Name != nil {
			w.function(fn, c, true, false)
			return
		}
		w.expr(t.Declaration, c)
	case *ImportDeclaration:
		for _, spec := range t.Specifiers {
			switch sp := spec.(type) {
			case *ImportDefaultSpecifier:
				w.binding(sp.Local, c)
			case *ImportNamespaceSpecifier:
				w.binding(sp.Local, c)
			case *ImportNamedSpecifier:
				if sp.Local != nil {
					w.binding(sp.Local, c)
				}
			}
		}
	}
}

// forHead handles the head of a for / for-in / for-of statement: either a
// declaration or an assignment target expression.
func (w *eeWalker) forHead(s Statement, c eeCtx) {
	if es, ok := s.(*ExpressionStatement); ok {
		switch es.Expression.(type) {
		case *Identifier, *ArrayLiteral, *ObjectLiteral, *ArrayDestructuringAssignment, *ObjectDestructuringAssignment:
			w.target(es.Expression, c)
			return
		}
	}
	w.stmt(s, c)
}

// --- expressions ---

func (w *eeWalker) exprs(list []Expression, c eeCtx) {
	for _, e := range list {
		w.expr(e, c)
	}
}

func (w *eeWalker) expr(e Expression, c eeCtx) {
	switch t := e.(type) {
	case nil:
	case *Identifier:
		w.ident(t, c)
	case *FunctionLiteral:
		w.function(t, c, false, false)
	case *ArrowFunctionLiteral:
		w.arrow(t, c)
	case *ClassExpression:
		if t.Name != nil {
			nc := c
			nc.strict = true
			w.binding(t.Name, nc)
		}
		w.class(t.SuperClass, t.Body, c)
	case *TemplateLiteral:
		for _, part := range t.Parts {
			if pe, ok := part.(Expression); ok {
				w.expr(pe, c)
			}
		}
	case *TaggedTemplateExpression:
		w.expr(t.Tag, c)
		if t.Template != nil {
			w.expr(t.Template, c)
		}
	case *AssignmentExpression:
		w.target(t.Left, c)
		w.expr(t.Value, c)
	case *ArrayDestructuringAssignment, *ObjectDestructuringAssignment:
		w.target(t, c)
	case *UpdateExpression:
		w.target(t.Argument, c)
	case *PrefixExpression:
		w.expr(t.Right, c)
	case *TypeofExpression:
		w.expr(t.Operand, c)
	case *InfixExpression:
		w.expr(t.Left, c)
		w.expr(t.Right, c)
	case *TernaryExpression:
		w.expr(t.Condition, c)
		w.expr(t.Consequence, c)
		w.expr(t.Alternative, c)
	case *CallExpression:
		w.expr(t.Function, c)
		w.exprs(t.Arguments, c)
	case *NewExpression:
		w.expr(t.Constructor, c)
		w.exprs(t.Arguments, c)
	case *IndexExpression:
		w.expr(t.Left, c)
		w.expr(t.Index, c)
	case *MemberExpression:
		w.expr(t.Object, c)
		if _, ok := t.Property.(*Identifier); !ok {
			w.expr(t.Property, c)
		}
	case *OptionalChainingExpression:
		w.expr(t.Object, c)
	case *OptionalIndexExpression:
		w.expr(t.Object, c)
		w.expr(t.Index, c)
	case *OptionalCallExpression:
		w.expr(t.Function, c)
		w.exprs(t.Arguments, c)
	case *SpreadElement:
		w.expr(t.Argument, c)
	case *YieldExpression:
		if !c.yield || c.noYieldExpr {
			w.errorf(t.Token, "Yield expression not allowed in this context")
		}
		w.expr(t.Value, c)
	case *AwaitExpression:
		if c.noAwaitExpr || c.staticBlock {
			w.errorf(t.Token, "Await expression not allowed in this context")
		}
		w.expr(t.Argument, c)
	case *TypeAssertionExpression:
		w.expr(t.Expression, c)
	case *SatisfiesExpression:
		w.expr(t.Expression, c)
	case *NonNullExpression:
		w.expr(t.Expression, c)
	case *ArrayLiteral:
		w.exprs(t.Elements, c)
	case *ObjectLiteral:
		for _, prop := range t.Properties {
			if prop == nil {
				continue
			}
			if isShorthand(prop) {
				// `{a}` references a; `{a = 1}` is only valid as a pattern.
				if ae, ok := prop.Value.(*AssignmentExpression); ok {
					w.errorf(ae.Token, "Invalid shorthand property initializer")
				}
				w.expr(prop.Value, c)
				continue
			}
			w.propKey(prop.Key, c)
			if md, ok := prop.Value.(*MethodDefinition); ok {
				if md.Value != nil {
					w.function(md.Value, c, false, true)
				}
				continue
			}
			w.expr(prop.Value, c)
		}
	case *ComputedPropertyName:
		w.expr(t.Expr, c)
	case *DynamicImportExpression:
		w.expr(t.Source, c)
		w.expr(t.Options, c)
	case *DeferredImportExpression:
		w.expr(t.Source, c)
	}
}

// --- functions and classes ---

// function checks a function declaration/expression/method.
func (w *eeWalker) function(fn *FunctionLiteral, outer eeCtx, isDecl, isMethod bool) {
	if fn == nil || fn.Body == nil {
		return
	}
	params := fn.Parameters
	stmts := fn.Body.Statements
	if fn.LoweredParams != nil {
		params = fn.LoweredParams
		if fn.LoweredStmts <= len(stmts) {
			stmts = stmts[fn.LoweredStmts:]
		}
	}

	inner := outer
	inner.yield = fn.IsGenerator
	inner.await = fn.IsAsync
	inner.staticBlock = false
	inner.noYieldExpr = false
	inner.noAwaitExpr = false
	directive := useStrictDirective(stmts)
	if directive != nil {
		inner.strict = true
	}

	if fn.Name != nil && !isMethod {
		// A declaration's name is bound in the enclosing scope (so [Yield]/
		// [Await] come from outside); an expression's name is bound inside.
		nc := outer
		if !isDecl {
			nc.yield, nc.await = inner.yield, inner.await
			nc.staticBlock = false
		}
		nc.strict = inner.strict
		w.binding(fn.Name, nc)
	}

	pc := inner
	pc.noYieldExpr = fn.IsGenerator
	pc.noAwaitExpr = fn.IsAsync
	w.params(params, fn.RestParameter, directive, pc, isMethod || inner.strict)
	w.stmts(stmts, inner)
}

func (w *eeWalker) arrow(fn *ArrowFunctionLiteral, outer eeCtx) {
	if fn == nil {
		return
	}
	params := fn.Parameters
	var stmts []Statement
	var exprBody Expression
	switch b := fn.Body.(type) {
	case *BlockStatement:
		stmts = b.Statements
	case Expression:
		exprBody = b
	}
	if fn.LoweredParams != nil {
		params = fn.LoweredParams
		if fn.LoweredStmts <= len(stmts) {
			stmts = stmts[fn.LoweredStmts:]
		}
	}

	// Arrow parameters see the enclosing [Yield]/[Await]; YieldExpression and
	// AwaitExpression are never allowed in them.
	pc := outer
	if fn.IsAsync {
		pc.await = true
	}
	pc.noYieldExpr = true
	pc.noAwaitExpr = pc.await

	inner := outer
	inner.yield = false
	inner.await = fn.IsAsync
	inner.staticBlock = false
	inner.noYieldExpr = false
	inner.noAwaitExpr = false
	var directive *lexer.Token
	if exprBody == nil {
		directive = useStrictDirective(stmts)
		if directive != nil {
			inner.strict = true
			pc.strict = true
		}
	}
	// Arrow parameters are always UniqueFormalParameters-like: no duplicates.
	w.params(params, fn.RestParameter, directive, pc, true)
	if exprBody != nil {
		w.expr(exprBody, inner)
	} else {
		w.stmts(stmts, inner)
	}
}

// params checks a formal parameter list. noDuplicates is true when the list
// must not contain duplicate names regardless of simplicity (strict code,
// methods, arrows).
func (w *eeWalker) params(params []*Parameter, rest *RestParameter, directive *lexer.Token, c eeCtx, noDuplicates bool) {
	simple := rest == nil
	for _, prm := range params {
		if prm != nil && (prm.IsDestructuring || prm.DefaultValue != nil) {
			simple = false
		}
	}
	if directive != nil && !simple {
		for _, prm := range params {
			if prm != nil && (prm.IsDestructuring || prm.DefaultValue != nil) && prm.Token != nil {
				w.p.addErrorWithCode(prm.Token, errors.TS1346, "This parameter is not allowed with 'use strict' directive.")
			}
		}
		if rest != nil && rest.Token != nil {
			w.p.addErrorWithCode(rest.Token, errors.TS1346, "This parameter is not allowed with 'use strict' directive.")
		}
		w.p.addErrorWithCode(directive, errors.TS1347, "'use strict' directive cannot be used with non-simple parameter list.")
	}

	var names []*Identifier
	saved := w.collect
	w.collect = &names
	for _, prm := range params {
		if prm == nil || prm.IsThis {
			continue
		}
		if prm.IsDestructuring {
			w.pattern(prm.Pattern, c)
		} else {
			w.binding(prm.Name, c)
		}
		w.expr(prm.DefaultValue, c)
	}
	if rest != nil {
		if rest.Pattern != nil {
			w.pattern(rest.Pattern, c)
		} else {
			w.binding(rest.Name, c)
		}
	}
	w.collect = saved

	if noDuplicates || !simple {
		seen := make(map[string]bool, len(names))
		for _, id := range names {
			if seen[id.Value] {
				w.p.addErrorWithCode(id.Token, errors.TS2300, "Duplicate identifier '"+id.Value+"'.")
			}
			seen[id.Value] = true
		}
	}
}

func (w *eeWalker) class(super Expression, body *ClassBody, outer eeCtx) {
	c := outer
	c.strict = true
	c.inClass = true
	w.expr(super, c)
	if body == nil {
		return
	}
	for _, m := range body.Methods {
		if m == nil {
			continue
		}
		w.propKey(m.Key, c)
		if m.Value != nil {
			w.function(m.Value, c, false, true)
		}
	}
	for _, prop := range body.Properties {
		if prop == nil {
			continue
		}
		w.propKey(prop.Key, c)
		// Field initializers are evaluated like method bodies: no [Yield] or
		// [Await], and `arguments` is forbidden.
		fc := c
		fc.yield, fc.await = false, false
		fc.staticBlock = false
		fc.noYieldExpr, fc.noAwaitExpr = false, false
		w.expr(prop.Value, fc)
	}
	for _, blk := range body.StaticInitializers {
		sc := c
		sc.yield, sc.await = false, false
		sc.staticBlock = true
		sc.noYieldExpr, sc.noAwaitExpr = false, false
		w.block(blk, sc)
	}
}
