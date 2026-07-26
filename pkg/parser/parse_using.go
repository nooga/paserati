package parser

import (
	"github.com/nooga/paserati/pkg/lexer"
)

// Explicit resource management (ES2026): `using x = expr` and
// `await using x = expr`.
//
// `using` is a contextual keyword — it stays a plain identifier everywhere
// except immediately before a binding name on the same line. The declarations
// are parsed into a LetStatement carrying IsUsing/IsAwaitUsing; the compiler
// lowers enclosing blocks into try/finally dispose calls from there.

// startsUsingDeclaration reports whether the current token begins a `using`
// declaration. Requires the literal `using` followed by a binding identifier on
// the same line — a LineTerminator makes ASI treat `using` as an expression,
// and `using of` is excluded so `for (using of xs)` still parses as for-of.
func (p *Parser) startsUsingDeclaration() bool {
	if p.curToken.Type != lexer.IDENT || p.curToken.Literal != "using" {
		return false
	}
	if p.peekToken.Line != p.curToken.Line {
		return false
	}
	if p.peekTokenIs(lexer.OF) {
		return false
	}
	return p.peekTokenIs(lexer.IDENT) || p.isKeywordThatCanBeIdentifier(p.peekToken.Type)
}

// startsAwaitUsingDeclaration reports whether the current token begins an
// `await using` declaration, i.e. `await` immediately followed by a `using`
// declaration on the same line.
func (p *Parser) startsAwaitUsingDeclaration() bool {
	if !p.curTokenIs(lexer.AWAIT) {
		return false
	}
	if !p.peekTokenIs(lexer.IDENT) || p.peekToken.Literal != "using" || p.peekToken.Line != p.curToken.Line {
		return false
	}
	// Look past `using` for a binding name; `of` would make this a for-of over
	// a variable named `using`.
	return !p.peekTokenIs2(lexer.OF)
}

// parseUsingStatement parses `using x = expr, y = expr;`. Current token is the
// `using` keyword. startToken is the token the statement is reported at — the
// `await` for an `await using`, otherwise `using` itself.
//
// Like `const`, every declarator requires an initializer; unlike `const`,
// destructuring patterns are not allowed as binding targets.
func (p *Parser) parseUsingStatement(startToken *lexer.Token, isAwait bool) Statement {
	stmt := &LetStatement{
		Token:        startToken,
		IsUsing:      true,
		IsAwaitUsing: isAwait,
	}

	for {
		if !p.expectPeekIdentifierOrKeyword() {
			return nil
		}

		declarator := &VarDeclarator{}
		declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

		if !p.parseDeclaratorAnnotation(declarator) {
			return nil
		}

		// A `using` declaration must be initialized — there is nothing to
		// dispose otherwise.
		if !p.expectPeek(lexer.ASSIGN) {
			return nil
		}
		p.nextToken() // Consume the token starting the expression
		declarator.Value = p.parseExpression(COMMA)

		stmt.Declarations = append(stmt.Declarations, declarator)

		if !p.peekTokenIs(lexer.COMMA) {
			break
		}
		p.nextToken() // Consume ','
	}

	// Legacy fields mirror the first declaration, as for let/const/var.
	if len(stmt.Declarations) > 0 {
		stmt.Name = stmt.Declarations[0].Name
		stmt.TypeAnnotation = stmt.Declarations[0].TypeAnnotation
		stmt.Value = stmt.Declarations[0].Value
		stmt.ComputedType = stmt.Declarations[0].ComputedType
	}

	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}
