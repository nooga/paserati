package parser

import (
	"paserati/pkg/lexer"
)

// parseEnumDeclarationStatement parses an enum declaration statement
func (p *Parser) parseEnumDeclarationStatement() *ExpressionStatement {
	enumDecl := p.parseEnumDeclaration(false) // false for regular enum
	if enumDecl == nil {
		return nil
	}
	
	return &ExpressionStatement{
		Token:      enumDecl.Token,
		Expression: enumDecl,
	}
}

// parseConstEnumDeclarationStatement parses a const enum declaration statement
func (p *Parser) parseConstEnumDeclarationStatement(constToken lexer.Token) *ExpressionStatement {
	enumDecl := p.parseEnumDeclaration(true) // true for const enum
	if enumDecl == nil {
		return nil
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
	if !p.curTokenIs(lexer.IDENT) {
		p.addError(p.curToken, "expected enum member name")
		return nil
	}
	
	memberToken := p.curToken
	name := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	
	var value Expression
	if p.peekTokenIs(lexer.ASSIGN) {
		p.nextToken() // consume '='
		p.nextToken() // move to value expression
		
		value = p.parseExpression(LOWEST)
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