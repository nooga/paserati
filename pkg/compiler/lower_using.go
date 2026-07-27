package compiler

import (
	"reflect"

	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
)

// Lowering for explicit resource management (ES2026).
//
// `using x = expr;` binds a resource whose disposal must run when the enclosing
// block exits, on every path — normal completion, `return`, `break`, `throw`.
// That is exactly `finally`, so rather than teach the code generator a new
// unwinding mechanism, each block containing `using` declarations is rewritten
// into the equivalent try/finally form before compilation:
//
//	{ before; using a = e; after; }
//
// becomes
//
//	{ before; let a = e; try { after } finally { a?.[Symbol.dispose]() } }
//
// Multiple resources nest, so a failure while acquiring the second still
// disposes the first, and disposal runs in reverse declaration order. This runs
// after type checking, so the checker still sees — and reports on — the
// original `using` declarations.

// lowerUsingDeclarations rewrites every statement list in the program that
// contains `using` declarations. It is a no-op for programs without any.
func lowerUsingDeclarations(program *parser.Program) {
	if program == nil {
		return
	}
	lowerUsingInNode(program)
}

// lowerUsingInNode rewrites node's own statement list (if it has one) after
// recursing into its children, so inner blocks are lowered first.
func lowerUsingInNode(node parser.Node) {
	// Guard against typed-nil interface values: optional AST fields declared as
	// Statement/Expression carry a (*T)(nil), which is non-nil at the interface
	// level but panics on field access.
	if node == nil {
		return
	}
	if v := reflect.ValueOf(node); v.Kind() == reflect.Ptr && v.IsNil() {
		return
	}
	switch n := node.(type) {
	case *parser.Program:
		for _, s := range n.Statements {
			lowerUsingInNode(s)
		}
		n.Statements = lowerUsingInStatements(n.Statements)
	case *parser.BlockStatement:
		for _, s := range n.Statements {
			lowerUsingInNode(s)
		}
		n.Statements = lowerUsingInStatements(n.Statements)
	case *parser.ExpressionStatement:
		lowerUsingInNode(n.Expression)
	case *parser.LetStatement:
		lowerUsingInDeclarators(n.Declarations)
	case *parser.ConstStatement:
		lowerUsingInDeclarators(n.Declarations)
	case *parser.VarStatement:
		lowerUsingInDeclarators(n.Declarations)
	case *parser.ReturnStatement:
		lowerUsingInNode(n.ReturnValue)
	case *parser.IfStatement:
		lowerUsingInNode(n.Condition)
		lowerUsingInNode(n.Consequence)
		lowerUsingInNode(n.Alternative)
	case *parser.WhileStatement:
		lowerUsingInNode(n.Body)
	case *parser.DoWhileStatement:
		lowerUsingInNode(n.Body)
	case *parser.ForStatement:
		lowerUsingInNode(n.Initializer)
		lowerUsingInNode(n.Body)
	case *parser.ForOfStatement:
		lowerUsingInNode(n.Body)
		lowerUsingInLoopVariable(n.Variable, n.Body)
	case *parser.ForInStatement:
		lowerUsingInNode(n.Body)
		lowerUsingInLoopVariable(n.Variable, n.Body)
	case *parser.LabeledStatement:
		lowerUsingInNode(n.Statement)
	case *parser.WithStatement:
		lowerUsingInNode(n.Body)
	case *parser.TryStatement:
		lowerUsingInNode(n.Body)
		if n.CatchClause != nil {
			lowerUsingInNode(n.CatchClause.Body)
		}
		lowerUsingInNode(n.FinallyBlock)
	case *parser.SwitchStatement:
		for _, sc := range n.Cases {
			if sc != nil {
				lowerUsingInNode(sc.Body)
			}
		}
	case *parser.FunctionLiteral:
		lowerUsingInNode(n.Body)
	case *parser.ArrowFunctionLiteral:
		lowerUsingInNode(n.Body)
	case *parser.ShorthandMethod:
		lowerUsingInNode(n.Body)
	case *parser.ClassDeclaration:
		lowerUsingInClassBody(n.Body)
	case *parser.ClassExpression:
		lowerUsingInClassBody(n.Body)
	case *parser.MethodDefinition:
		if n.Value != nil {
			lowerUsingInNode(n.Value)
		}
	case *parser.NamespaceDeclaration:
		lowerUsingInNode(n.Body)
	case *parser.ExportNamedDeclaration:
		lowerUsingInNode(n.Declaration)
	case *parser.ExportDefaultDeclaration:
		lowerUsingInNode(n.Declaration)
	}
}

func lowerUsingInDeclarators(decls []*parser.VarDeclarator) {
	for _, d := range decls {
		if d != nil {
			lowerUsingInNode(d.Value)
		}
	}
}

func lowerUsingInClassBody(body *parser.ClassBody) {
	if body == nil {
		return
	}
	for _, m := range body.Methods {
		if m != nil && m.Value != nil {
			lowerUsingInNode(m.Value)
		}
	}
	for _, b := range body.StaticInitializers {
		lowerUsingInNode(b)
	}
}

// lowerUsingInLoopVariable handles `for (using d of xs)`. The resource is
// disposed at the end of every iteration, so the binding becomes an ordinary
// one and the loop body is wrapped in a try/finally.
func lowerUsingInLoopVariable(variable parser.Statement, body *parser.BlockStatement) {
	ls, ok := variable.(*parser.LetStatement)
	if !ok || !ls.IsUsing || body == nil || len(ls.Declarations) == 0 {
		return
	}
	d := ls.Declarations[0]
	if d == nil || d.Name == nil {
		return
	}
	inner := &parser.BlockStatement{Token: body.Token, Statements: body.Statements}
	body.Statements = []parser.Statement{
		&parser.TryStatement{
			Token: ls.Token,
			Body:  inner,
			FinallyBlock: &parser.BlockStatement{
				Token:      ls.Token,
				Statements: []parser.Statement{buildDisposeStatement(ls, d.Name)},
			},
		},
	}
	// The binding itself is now an ordinary per-iteration one.
	ls.IsUsing = false
	ls.IsAwaitUsing = false
}

// lowerUsingInStatements splits a statement list at its first `using`
// declaration and wraps everything after it in a try/finally that disposes the
// resource. Statements after the split are lowered recursively, so several
// `using` declarations in one block nest from the outside in.
func lowerUsingInStatements(stmts []parser.Statement) []parser.Statement {
	for i, s := range stmts {
		if forStmt, ok := s.(*parser.ForStatement); ok {
			if lowered := lowerUsingInForInitializer(forStmt); lowered != nil {
				out := make([]parser.Statement, 0, len(stmts))
				out = append(out, stmts[:i]...)
				out = append(out, lowered)
				return append(out, lowerUsingInStatements(stmts[i+1:])...)
			}
			continue
		}
		ls, ok := s.(*parser.LetStatement)
		if !ok || !ls.IsUsing {
			continue
		}
		rest := lowerUsingInStatements(stmts[i+1:])
		lowered := make([]parser.Statement, 0, i+2)
		lowered = append(lowered, stmts[:i]...)
		return append(lowered, buildUsingScope(ls, 0, rest)...)
	}
	return stmts
}

// lowerUsingInForInitializer handles `for (using d = e; cond; upd) body`, where
// the resource lives for the whole loop. The binding moves ahead of the loop
// and the loop becomes the body of a try/finally, all inside a fresh block so
// the binding stays scoped to the statement. Returns nil if the loop head holds
// no `using` declaration.
func lowerUsingInForInitializer(forStmt *parser.ForStatement) parser.Statement {
	ls, ok := forStmt.Initializer.(*parser.LetStatement)
	if !ok || !ls.IsUsing {
		return nil
	}
	forStmt.Initializer = nil
	return &parser.BlockStatement{
		Token:      ls.Token,
		Statements: buildUsingScope(ls, 0, []parser.Statement{forStmt}),
	}
}

// buildUsingScope emits the binding for declarator idx followed by a
// try/finally guarding everything that comes after it. Declarators past idx are
// nested inside that try, which gives reverse-order disposal and keeps earlier
// resources protected if a later acquisition throws.
func buildUsingScope(ls *parser.LetStatement, idx int, rest []parser.Statement) []parser.Statement {
	if idx >= len(ls.Declarations) {
		return rest
	}
	d := ls.Declarations[idx]
	if d == nil || d.Name == nil {
		return buildUsingScope(ls, idx+1, rest)
	}

	binding := &parser.LetStatement{
		Token:          ls.Token,
		Declarations:   []*parser.VarDeclarator{d},
		Name:           d.Name,
		TypeAnnotation: d.TypeAnnotation,
		Value:          d.Value,
		ComputedType:   d.ComputedType,
	}

	// Per spec the dispose method is looked up when the declaration is
	// evaluated, not when disposal runs, so a non-disposable resource must
	// throw right here.
	guard := buildDisposableGuard(ls, d.Name)

	guarded := &parser.TryStatement{
		Token: ls.Token,
		Body: &parser.BlockStatement{
			Token:      ls.Token,
			Statements: buildUsingScope(ls, idx+1, rest),
		},
		FinallyBlock: &parser.BlockStatement{
			Token:      ls.Token,
			Statements: []parser.Statement{buildDisposeStatement(ls, d.Name)},
		},
	}

	return []parser.Statement{binding, guard, guarded}
}

// buildDisposableGuard produces
//
//	if (name !== null && name !== undefined && typeof name[Symbol.dispose] !== "function") {
//	    throw new TypeError("...");
//	}
//
// For `await using`, the resource qualifies if it has either
// `Symbol.asyncDispose` or `Symbol.dispose`.
func buildDisposableGuard(ls *parser.LetStatement, name *parser.Identifier) parser.Statement {
	tok := ls.Token

	missing := notCallableMethod(tok, name, "dispose")
	if ls.IsAwaitUsing {
		missing = &parser.InfixExpression{
			Token:    tok,
			Left:     notCallableMethod(tok, name, "asyncDispose"),
			Operator: "&&",
			Right:    missing,
		}
	}

	message := "Object is not disposable."
	if ls.IsAwaitUsing {
		message = "Object is not async disposable."
	}

	return &parser.IfStatement{
		Token: tok,
		Condition: &parser.InfixExpression{
			Token:    tok,
			Left:     buildPresenceCheck(tok, name),
			Operator: "&&",
			Right:    missing,
		},
		Consequence: &parser.BlockStatement{
			Token: tok,
			Statements: []parser.Statement{
				&parser.ThrowStatement{
					Token: tok,
					Value: &parser.NewExpression{
						Token:       tok,
						Constructor: synthIdentifier(tok, "TypeError"),
						Arguments:   []parser.Expression{&parser.StringLiteral{Token: tok, Value: message}},
					},
				},
			},
		},
	}
}

// notCallableMethod builds `typeof name[Symbol.<method>] !== "function"`.
func notCallableMethod(tok *lexer.Token, name *parser.Identifier, method string) parser.Expression {
	return &parser.InfixExpression{
		Token: tok,
		Left: &parser.TypeofExpression{
			Token:   tok,
			Operand: buildSymbolMethodAccess(tok, name, method),
		},
		Operator: "!==",
		Right:    &parser.StringLiteral{Token: tok, Value: "function"},
	}
}

// buildSymbolMethodAccess builds `name[Symbol.<method>]`.
func buildSymbolMethodAccess(tok *lexer.Token, name *parser.Identifier, method string) parser.Expression {
	return &parser.IndexExpression{
		Token: tok,
		Left:  synthIdentifier(tok, name.Value),
		Index: &parser.MemberExpression{
			Token:    tok,
			Object:   synthIdentifier(tok, "Symbol"),
			Property: synthIdentifier(tok, method),
		},
	}
}

// buildPresenceCheck builds `name !== null && name !== undefined`.
func buildPresenceCheck(tok *lexer.Token, name *parser.Identifier) parser.Expression {
	return &parser.InfixExpression{
		Token:    tok,
		Operator: "&&",
		Left: &parser.InfixExpression{
			Token:    tok,
			Left:     synthIdentifier(tok, name.Value),
			Operator: "!==",
			Right:    &parser.NullLiteral{Token: tok},
		},
		Right: &parser.InfixExpression{
			Token:    tok,
			Left:     synthIdentifier(tok, name.Value),
			Operator: "!==",
			Right:    &parser.UndefinedLiteral{Token: tok},
		},
	}
}

// buildDisposeStatement produces
//
//	if (name !== null && name !== undefined) { name[Symbol.dispose](); }
//
// For `await using` the call is awaited and prefers `Symbol.asyncDispose`,
// falling back to `Symbol.dispose`. Null and undefined resources are legal and
// dispose to nothing, per spec.
func buildDisposeStatement(ls *parser.LetStatement, name *parser.Identifier) parser.Statement {
	tok := ls.Token

	var call parser.Expression = disposeCall(tok, name, "dispose")
	if ls.IsAwaitUsing {
		call = &parser.AwaitExpression{
			Token: tok,
			Argument: &parser.TernaryExpression{
				Token:       tok,
				Condition:   notCallableMethod(tok, name, "asyncDispose"),
				Consequence: disposeCall(tok, name, "dispose"),
				Alternative: disposeCall(tok, name, "asyncDispose"),
			},
		}
	}

	return &parser.IfStatement{
		Token:     tok,
		Condition: buildPresenceCheck(tok, name),
		Consequence: &parser.BlockStatement{
			Token:      tok,
			Statements: []parser.Statement{&parser.ExpressionStatement{Token: tok, Expression: call}},
		},
	}
}

// disposeCall builds `name[Symbol.<method>]()`.
func disposeCall(tok *lexer.Token, name *parser.Identifier, method string) parser.Expression {
	return &parser.CallExpression{
		Token:    tok,
		Function: buildSymbolMethodAccess(tok, name, method),
	}
}

func synthIdentifier(tok *lexer.Token, name string) *parser.Identifier {
	return &parser.Identifier{Token: tok, Value: name}
}
