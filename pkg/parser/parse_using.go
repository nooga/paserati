package parser

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
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
//
// Unlike startsUsingDeclarationForLoop, this requires an actual identifier:
// at statement level `using [a] = null;` is not a using declaration with an
// invalid pattern at all — real TypeScript parses it as the member
// expression `using[a] = null`, since nothing here forces `using` to be a
// declaration keyword the way a for-loop head does.
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

// isUsingOfDisambiguator reports whether a token following `using of` (or
// `await using of`) in a for-loop head proves that `of` is being used as a
// binding name, not the for-of separator: a for-of iterable expression can
// never start with `=` or `:` (a TS type annotation on the binding), so
// either one unambiguously means this is a using-declaration.
func isUsingOfDisambiguator(t lexer.TokenType) bool {
	return t == lexer.ASSIGN || t == lexer.COLON
}

// startsUsingDeclarationForLoop is startsUsingDeclaration's for-loop-head
// counterpart: a for-of/for-in head unambiguously expects a binding target,
// so `for (using {} of xs)` does commit to a using declaration even though
// `{}` isn't a valid one — TypeScript reports TS1492 for it rather than
// treating `using` as a plain identifier. Called only after the caller has
// already advanced onto the `using` token itself.
func (p *Parser) startsUsingDeclarationForLoop() bool {
	if p.curToken.Type != lexer.IDENT || p.curToken.Literal != "using" {
		return false
	}
	if p.peekToken.Line != p.curToken.Line {
		return false
	}
	if p.peekTokenIs(lexer.OF) {
		// `using of` is ambiguous between a for-of loop over a variable named
		// `using` (`for (using of xs)`) and a using-declaration whose binding
		// name is literally `of` in a classic for-statement
		// (`for (using of = e;;)`) - `of` isn't restricted as a for-loop
		// binding name outside the for-of/for-in head itself.
		return isUsingOfDisambiguator(p.lookAhead(1).Type)
	}
	return p.peekTokenIs(lexer.IDENT) || p.isKeywordThatCanBeIdentifier(p.peekToken.Type) ||
		p.peekTokenIs(lexer.LBRACKET) || p.peekTokenIs(lexer.LBRACE)
}

// startsUsingDeclarationAt reports whether a `using` declaration begins at
// lookahead position `at`, where 0 is the peek token. Used from a for-loop head,
// where the decision must be made before advancing into the parenthesis.
//
// Requires `using` followed by a binding name, so `for (using of xs)` (a for-of
// over a variable named `using`) and `for (await foo; ...)` (an await
// expression) are both correctly left alone - except `using of =`, see
// startsUsingDeclarationForLoop.
func (p *Parser) startsUsingDeclarationAt(at int) bool {
	tok := p.lookAhead(at)
	if tok.Type != lexer.IDENT || tok.Literal != "using" {
		return false
	}
	name := p.lookAhead(at + 1)
	if name.Line != tok.Line {
		return false
	}
	if name.Type == lexer.OF {
		return isUsingOfDisambiguator(p.lookAhead(at + 2).Type)
	}
	return name.Type == lexer.IDENT || p.isKeywordThatCanBeIdentifier(name.Type) ||
		name.Type == lexer.LBRACKET || name.Type == lexer.LBRACE
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
	// a variable named `using` - unless followed by `=`/`:`, see
	// startsUsingDeclarationForLoop.
	if p.peekTokenIs2(lexer.OF) {
		return isUsingOfDisambiguator(p.lookAhead(2).Type)
	}
	return true
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
		var declarator *VarDeclarator
		if p.peekTokenIs(lexer.LBRACKET) || p.peekTokenIs(lexer.LBRACE) {
			// A binding pattern isn't a valid `using` target — there is no
			// single resource to dispose. Report it, then parse the pattern
			// anyway (reusing the parameter-pattern parser purely to consume
			// it correctly) so the rest of the declaration, and any
			// statements after it, still parse.
			p.nextToken() // Move onto '[' or '{'
			patternToken := p.curToken
			kind := "'using'"
			if isAwait {
				kind = "'await using'"
			}
			p.addErrorWithCode(patternToken, errors.TS1492, fmt.Sprintf("%s declarations may not have binding patterns.", kind))
			if patternToken.Type == lexer.LBRACKET {
				p.parseArrayParameterPattern()
			} else {
				p.parseObjectParameterPattern()
			}
			declarator = &VarDeclarator{Name: &Identifier{Token: patternToken, Value: "<pattern>"}}
		} else {
			if !p.expectPeekIdentifierOrKeyword() {
				return nil
			}
			declarator = &VarDeclarator{}
			declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		}

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
