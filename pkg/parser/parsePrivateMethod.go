package parser

import "paserati/pkg/lexer"

// parsePrivateMethod parses a private method definition like #methodName() { ... }
func (p *Parser) parsePrivateMethod(isStatic bool) *MethodDefinition {
	methodToken := p.curToken
	// Create identifier from PRIVATE_IDENT token (includes the '#')
	methodName := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	
	p.nextToken() // move past private method name - should now be at '('
	
	// Check that we're at the '(' token - parseFunctionParameters expects us to be AT the '(' token
	if !p.curTokenIs(lexer.LPAREN) {
		p.addError(p.curToken, "expected '(' after private method name")
		return nil
	}
	
	// Use existing parameter parsing function
	parameters, restParameter, err := p.parseFunctionParameters(false)
	if err != nil {
		return nil
	}
	
	// Parse optional return type annotation
	var returnTypeAnnotation Expression
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Move to ':'
		p.nextToken() // Move past ':'
		returnTypeAnnotation = p.parseTypeExpression()
	}
	
	// Parse method body
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}
	
	body := p.parseBlockStatement()
	if body == nil {
		return nil
	}
	
	// parseBlockStatement leaves us at '}', advance past it
	p.nextToken()
	
	// Create function literal for the method
	funcLiteral := &FunctionLiteral{
		Token:                methodToken,
		Parameters:           parameters,
		RestParameter:        restParameter,
		ReturnTypeAnnotation: returnTypeAnnotation,
		Body:                 body,
	}
	
	// Create method definition
	method := &MethodDefinition{
		Token:    methodToken,
		Key:      methodName,
		Value:    funcLiteral,
		Kind:     "method", // Regular private method
		IsStatic: isStatic,
		// Private methods are implicitly private, but we don't have separate access modifiers
		// The privacy is indicated by the '#' in the method name
	}
	
	return method
}