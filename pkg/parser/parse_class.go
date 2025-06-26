package parser

import (
	"fmt"
	"paserati/pkg/lexer"
)

// parseClassDeclaration parses a class declaration statement
// Syntax: class ClassName [extends SuperClass] { classBody }
func (p *Parser) parseClassDeclaration() Statement {
	classToken := p.curToken
	
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	
	name := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	
	var superClass *Identifier
	if p.peekTokenIs(lexer.EXTENDS) {
		p.nextToken() // consume 'extends'
		if !p.expectPeek(lexer.IDENT) {
			return nil
		}
		superClass = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}
	
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}
	
	body := p.parseClassBody()
	if body == nil {
		return nil
	}
	
	// parseClassBody leaves us at the '}' token, don't advance past it
	// Let the main parsing loop handle advancing to the next token
	
	return &ClassDeclaration{
		Token:      classToken,
		Name:       name,
		SuperClass: superClass,
		Body:       body,
	}
}

// parseClassExpression parses a class expression
// Syntax: class [ClassName] [extends SuperClass] { classBody }
func (p *Parser) parseClassExpression() Expression {
	classToken := p.curToken
	
	var name *Identifier
	if p.peekTokenIs(lexer.IDENT) {
		p.nextToken()
		name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}
	
	var superClass *Identifier
	if p.peekTokenIs(lexer.EXTENDS) {
		p.nextToken() // consume 'extends'
		if !p.expectPeek(lexer.IDENT) {
			return nil
		}
		superClass = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}
	
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}
	
	body := p.parseClassBody()
	if body == nil {
		return nil
	}
	
	// parseClassBody leaves us at the '}' token, don't advance past it
	// Let the main parsing loop handle advancing to the next token
	
	return &ClassExpression{
		Token:      classToken,
		Name:       name,
		SuperClass: superClass,
		Body:       body,
	}
}

// parseClassBody parses the body of a class containing methods and properties
func (p *Parser) parseClassBody() *ClassBody {
	bodyToken := p.curToken // The '{' token
	
	var methods []*MethodDefinition
	var properties []*PropertyDefinition
	
	p.nextToken() // move past '{'
	
	for !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
		// Skip semicolons
		if p.curTokenIs(lexer.SEMICOLON) {
			p.nextToken()
			continue
		}
		
		// Check for static modifier (for future use)
		isStatic := false
		if p.curTokenIs(lexer.STATIC) {
			isStatic = true
			p.nextToken()
		}
		
		// Parse constructor or method
		if p.curTokenIs(lexer.IDENT) {
			if p.curToken.Literal == "constructor" {
				method := p.parseConstructor(isStatic)
				if method != nil {
					methods = append(methods, method)
				}
			} else {
				// Could be a method or property
				// Look ahead to distinguish
				if p.peekTokenIs(lexer.LPAREN) {
					// It's a method
					method := p.parseMethod(isStatic)
					if method != nil {
						methods = append(methods, method)
					}
				} else {
					// It's a property
					property := p.parseProperty(isStatic)
					if property != nil {
						properties = append(properties, property)
					}
				}
			}
		} else {
			p.addError(p.curToken, "expected identifier in class body")
			p.nextToken()
		}
	}
	
	if !p.curTokenIs(lexer.RBRACE) {
		p.addError(p.curToken, "expected '}' to close class body")
		return nil
	}
	
	// The class body parsing is complete. We don't advance past the '}' here 
	// to be consistent with other parsing functions like parseBlockStatement
	
	return &ClassBody{
		Token:      bodyToken,
		Methods:    methods,
		Properties: properties,
	}
}

// parseConstructor parses a constructor method
func (p *Parser) parseConstructor(isStatic bool) *MethodDefinition {
	constructorToken := p.curToken
	
	// We're currently at the "constructor" token, next should be (
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}
	
	// Create a function literal structure manually
	functionLiteral := &FunctionLiteral{
		Token: constructorToken,
		Name:  nil, // Constructors don't have names like regular functions
	}
	
	// Parse parameters - we're now at the '(' token
	var err error
	functionLiteral.Parameters, functionLiteral.RestParameter, err = p.parseFunctionParameters()
	if err != nil {
		p.addError(p.curToken, fmt.Sprintf("failed to parse constructor parameters: %s", err.Error()))
		return nil
	}
	
	// Parse return type annotation if present
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // consume ':'
		p.nextToken() // move to start of type expression
		
		// Parse the return type using existing type parsing logic
		functionLiteral.ReturnTypeAnnotation = p.parseTypeExpression()
		if functionLiteral.ReturnTypeAnnotation == nil {
			return nil
		}
	}
	
	// Parse body - after parseFunctionParameters we should be at ')' with peek being '{' 
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}
	
	functionLiteral.Body = p.parseBlockStatement()
	
	// parseBlockStatement leaves us at '}', advance past it
	p.nextToken()
	
	return &MethodDefinition{
		Token:    constructorToken,
		Key:      &Identifier{Token: constructorToken, Value: "constructor"},
		Value:    functionLiteral,
		Kind:     "constructor",
		IsStatic: isStatic,
	}
}

// parseMethod parses a regular method
func (p *Parser) parseMethod(isStatic bool) *MethodDefinition {
	methodToken := p.curToken
	methodName := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	
	// We're currently at the method name token, next should be (
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}
	
	// Create a function literal structure manually
	functionLiteral := &FunctionLiteral{
		Token: methodToken,
		Name:  nil, // Methods don't have names in the traditional function sense
	}
	
	// Parse parameters - we're now at the '(' token
	var err error
	functionLiteral.Parameters, functionLiteral.RestParameter, err = p.parseFunctionParameters()
	if err != nil {
		p.addError(p.curToken, fmt.Sprintf("failed to parse method parameters: %s", err.Error()))
		return nil
	}
	
	// Parse return type annotation if present
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // consume ':'
		p.nextToken() // move to start of type expression
		
		// Parse the return type using existing type parsing logic
		functionLiteral.ReturnTypeAnnotation = p.parseTypeExpression()
		if functionLiteral.ReturnTypeAnnotation == nil {
			return nil
		}
	}
	
	// Parse body - after parseFunctionParameters we should be at ')' with peek being '{'
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}
	
	functionLiteral.Body = p.parseBlockStatement()
	
	// parseBlockStatement leaves us at '}', advance past it
	p.nextToken()
	
	return &MethodDefinition{
		Token:    methodToken,
		Key:      methodName,
		Value:    functionLiteral,
		Kind:     "method",
		IsStatic: isStatic,
	}
}

// parseProperty parses a property declaration
func (p *Parser) parseProperty(isStatic bool) *PropertyDefinition {
	propertyToken := p.curToken
	propertyName := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	
	p.nextToken() // move past property name
	
	// Check for optional marker '?' first
	var isOptional bool
	if p.curTokenIs(lexer.QUESTION) {
		isOptional = true
		p.nextToken() // Consume '?'
	}
	
	// Parse optional type annotation using interface pattern
	var typeAnnotation Expression
	if p.curTokenIs(lexer.COLON) {
		// Already at ':' token, move to type expression
		p.nextToken() // Move to the start of the type expression
		
		// Parse type
		typeAnnotation = p.parseTypeExpression()
		if typeAnnotation == nil {
			return nil
		}
		
		// After parsing type, advance to next token
		p.nextToken()
	}
	
	var initializer Expression
	if p.curTokenIs(lexer.ASSIGN) {
		p.nextToken() // move past '='
		initializer = p.parseExpression(LOWEST)
	}
	
	// Expect semicolon or end of class body
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	
	return &PropertyDefinition{
		Token:          propertyToken,
		Key:            propertyName,
		TypeAnnotation: typeAnnotation,
		Value:          initializer,
		IsStatic:       isStatic,
		Optional:       isOptional,
	}
}