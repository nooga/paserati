package parser

import (
	"github.com/nooga/paserati/pkg/lexer"
)

// parseEnumDeclarationStatement parses an enum declaration statement
func (p *Parser) parseEnumDeclarationStatement() *ExpressionStatement {
	enumDecl := p.parseEnumDeclaration(false) // false for regular enum
	if enumDecl == nil {
		// If enum parsing failed (e.g. `enum` reached in a position that
		// isn't actually a declaration - see paserati#426 Symptom 2, where
		// an invalid member-access continuation like `.enum` inside a
		// deeply nested expression re-synced the parser onto the bare
		// `enum` token, which this function then failed to turn into a
		// declaration), return a non-nil empty ExpressionStatement instead
		// of a literal nil. A literal `nil` here is a *ExpressionStatement,
		// which is a non-nil `Statement` interface value once returned
		// through parseStatement()'s dispatch (the classic Go typed-nil-in-
		// interface trap) - callers checking `stmt != nil` then see "true"
		// and proceed to dereference a nil ExpressionStatement, panicking.
		// Mirrors the same defensive pattern already used by
		// parseFunctionDeclarationStatement/parseAsyncFunctionDeclarationStatement
		// a few hundred lines up in parser.go, for the identical reason.
		return &ExpressionStatement{
			Token:      p.curToken,
			Expression: nil,
		}
	}

	return &ExpressionStatement{
		Token:      enumDecl.Token,
		Expression: enumDecl,
	}
}

// parseConstEnumDeclarationStatement parses a const enum declaration statement
func (p *Parser) parseConstEnumDeclarationStatement(constToken *lexer.Token) *ExpressionStatement {
	enumDecl := p.parseEnumDeclaration(true) // true for const enum
	if enumDecl == nil {
		// See the identical comment in parseEnumDeclarationStatement above.
		return &ExpressionStatement{
			Token:      constToken,
			Expression: nil,
		}
	}

	// Set the const token as the main token (for error reporting)
	enumDecl.Token = constToken
	enumDecl.IsConst = true

	return &ExpressionStatement{
		Token:      constToken,
		Expression: enumDecl,
	}
}

// parseEnumDeclaration parses an enum declaration
// Syntax: enum Name { Member1, Member2 = value, Member3 }
func (p *Parser) parseEnumDeclaration(isConst bool) *EnumDeclaration {
	enumToken := p.curToken

	if !p.expectPeek(lexer.IDENT) {
		return nil
	}

	name := &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	members := []*EnumMember{}

	// Parse enum members
	if !p.peekTokenIs(lexer.RBRACE) {
		p.nextToken() // Move to first member

		for {
			member := p.parseEnumMember()
			if member == nil {
				return nil
			}
			members = append(members, member)

			if p.peekTokenIs(lexer.RBRACE) {
				break
			}

			if !p.expectPeek(lexer.COMMA) {
				return nil
			}

			// Handle trailing comma
			if p.peekTokenIs(lexer.RBRACE) {
				break
			}

			p.nextToken() // Move to next member
		}
	}

	if !p.expectPeek(lexer.RBRACE) {
		return nil
	}

	return &EnumDeclaration{
		Token:   enumToken,
		Name:    name,
		Members: members,
		IsConst: isConst,
	}
}

// parseEnumMember parses a single enum member
// Syntax: MemberName [= value]
func (p *Parser) parseEnumMember() *EnumMember {
	// Enum members can be identifiers, keywords (used as names), or string literals
	var memberToken *lexer.Token
	var name *Identifier

	if p.curTokenIs(lexer.STRING) {
		// String literal member name: enum E { "A", "B" }
		memberToken = p.curToken
		name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	} else if p.curTokenIs(lexer.IDENT) || p.isKeywordThatCanBeIdentifier(p.curToken.Type) {
		// Identifier or keyword used as member name: enum E { interface, delete }
		memberToken = p.curToken
		name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	} else {
		p.addError(p.curToken, "expected enum member name")
		return nil
	}

	var value Expression
	if p.peekTokenIs(lexer.ASSIGN) {
		p.nextToken() // consume '='
		p.nextToken() // move to value expression

		value = p.parseExpression(COMMA)
		if value == nil {
			p.addError(p.curToken, "expected enum member value after '='")
			return nil
		}
	}

	return &EnumMember{
		Token: memberToken,
		Name:  name,
		Value: value,
	}
}
