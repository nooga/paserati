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
	
	// Parse type parameters if present (same pattern as interfaces)
	typeParameters := p.tryParseTypeParameters()
	
	var superClass Expression
	if p.peekTokenIs(lexer.EXTENDS) {
		p.nextToken() // consume 'extends'
		p.nextToken() // move to start of type expression
		
		// Parse full type expression (supports both simple identifiers and generic types)
		superClass = p.parseTypeExpression()
		if superClass == nil {
			return nil // Failed to parse superclass type
		}
	}
	
	var implements []*Identifier
	if p.peekTokenIs(lexer.IMPLEMENTS) {
		p.nextToken() // consume 'implements'
		
		// Parse comma-separated list of interface names
		for {
			if !p.expectPeek(lexer.IDENT) {
				return nil
			}
			implements = append(implements, &Identifier{Token: p.curToken, Value: p.curToken.Literal})
			
			if !p.peekTokenIs(lexer.COMMA) {
				break
			}
			p.nextToken() // consume ','
		}
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
		Token:          classToken,
		Name:           name,
		TypeParameters: typeParameters,
		SuperClass:     superClass,
		Implements:     implements,
		Body:           body,
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
	
	// Parse type parameters if present (same pattern as interfaces)
	// Note: Anonymous classes (name == nil) can still have type parameters
	typeParameters := p.tryParseTypeParameters()
	
	var superClass Expression
	if p.peekTokenIs(lexer.EXTENDS) {
		p.nextToken() // consume 'extends'
		p.nextToken() // move to start of type expression
		
		// Parse full type expression (supports both simple identifiers and generic types)
		superClass = p.parseTypeExpression()
		if superClass == nil {
			return nil // Failed to parse superclass type
		}
	}
	
	var implements []*Identifier
	if p.peekTokenIs(lexer.IMPLEMENTS) {
		p.nextToken() // consume 'implements'
		
		// Parse comma-separated list of interface names
		for {
			if !p.expectPeek(lexer.IDENT) {
				return nil
			}
			implements = append(implements, &Identifier{Token: p.curToken, Value: p.curToken.Literal})
			
			if !p.peekTokenIs(lexer.COMMA) {
				break
			}
			p.nextToken() // consume ','
		}
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
		Token:          classToken,
		Name:           name,
		TypeParameters: typeParameters,
		SuperClass:     superClass,
		Implements:     implements,
		Body:           body,
	}
}

// parseClassBody parses the body of a class containing methods and properties
func (p *Parser) parseClassBody() *ClassBody {
	bodyToken := p.curToken // The '{' token
	
	var methods []*MethodDefinition
	var properties []*PropertyDefinition
	var constructorSigs []*ConstructorSignature
	var methodSigs []*MethodSignature
	
	p.nextToken() // move past '{'
	
	for !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
		// Skip semicolons
		if p.curTokenIs(lexer.SEMICOLON) {
			p.nextToken()
			continue
		}
		
		// Parse modifiers in any order
		isReadonly := false
		isStatic := false
		isPublic := false
		isPrivate := false
		isProtected := false
		isAbstract := false
		isOverride := false
		
		// Parse all modifiers until we hit something that's not a modifier
		for {
			if p.curTokenIs(lexer.READONLY) && !isReadonly {
				isReadonly = true
				p.nextToken()
			} else if p.curTokenIs(lexer.STATIC) && !isStatic {
				isStatic = true
				p.nextToken()
			} else if p.curTokenIs(lexer.PUBLIC) && !isPublic && !isPrivate && !isProtected {
				isPublic = true
				p.nextToken()
			} else if p.curTokenIs(lexer.PRIVATE) && !isPublic && !isPrivate && !isProtected {
				isPrivate = true
				p.nextToken()
			} else if p.curTokenIs(lexer.PROTECTED) && !isPublic && !isPrivate && !isProtected {
				isProtected = true
				p.nextToken()
			} else if p.curTokenIs(lexer.ABSTRACT) && !isAbstract {
				isAbstract = true
				p.nextToken()
			} else if p.curTokenIs(lexer.OVERRIDE) && !isOverride {
				isOverride = true
				p.nextToken()
			} else {
				break // No more modifiers
			}
		}
		
		// Parse constructor, method, getter, or setter
		if p.curTokenIs(lexer.GET) {
			// Parse getter method
			method := p.parseGetter(isStatic, isPublic, isPrivate, isProtected, isOverride)
			if method != nil {
				methods = append(methods, method)
			}
		} else if p.curTokenIs(lexer.SET) {
			// Parse setter method
			method := p.parseSetter(isStatic, isPublic, isPrivate, isProtected, isOverride)
			if method != nil {
				methods = append(methods, method)
			}
		} else if p.curTokenIs(lexer.IDENT) {
			if p.curToken.Literal == "constructor" {
				// Parse constructor - let it decide if it's signature or implementation
				result := p.parseConstructor(isStatic, isPublic, isPrivate, isProtected)
				if result != nil {
					if sig, ok := result.(*ConstructorSignature); ok {
						// It's a signature
						constructorSigs = append(constructorSigs, sig)
					} else if method, ok := result.(*MethodDefinition); ok {
						// It's an implementation
						methods = append(methods, method)
					}
				}
			} else {
				// Could be a method or property
				// Look ahead to distinguish
				if p.peekTokenIs(lexer.LPAREN) || p.peekTokenIs(lexer.LT) {
					// It's a method (either regular or generic) - parse it and handle signatures/implementations
					result := p.parseMethod(isStatic, isPublic, isPrivate, isProtected, isAbstract, isOverride)
					if result != nil {
						if sig, ok := result.(*MethodSignature); ok {
							// It's a method signature
							methodSigs = append(methodSigs, sig)
						} else if method, ok := result.(*MethodDefinition); ok {
							// It's a method implementation
							methods = append(methods, method)
						}
					}
				} else {
					// It's a property
					property := p.parseProperty(isStatic, isReadonly, isPublic, isPrivate, isProtected)
					if property != nil {
						properties = append(properties, property)
					}
				}
			}
		} else if p.curTokenIs(lexer.LBRACKET) {
			// Computed property or method: [expr]: type or [expr]() {}
			result := p.parseComputedClassMember(isStatic, isReadonly, isPublic, isPrivate, isProtected, isAbstract, isOverride)
			if result != nil {
				if sig, ok := result.(*MethodSignature); ok {
					methodSigs = append(methodSigs, sig)
				} else if method, ok := result.(*MethodDefinition); ok {
					methods = append(methods, method)
				} else if property, ok := result.(*PropertyDefinition); ok {
					properties = append(properties, property)
				}
			}
		} else {
			p.addError(p.curToken, "expected identifier, 'get', 'set', or '[' in class body")
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
		Token:           bodyToken,
		Methods:         methods,
		Properties:      properties,
		ConstructorSigs: constructorSigs,
		MethodSigs:      methodSigs,
	}
}

// parseConstructor parses a constructor method or signature
// Returns either *ConstructorSignature or *MethodDefinition based on whether it ends with ';' or '{'
func (p *Parser) parseConstructor(isStatic, isPublic, isPrivate, isProtected bool) interface{} {
	constructorToken := p.curToken
	
	// Try to parse type parameters: constructor<T>()
	typeParameters := p.tryParseTypeParameters()
	
	// We're currently at the "constructor" token (or after type params), next should be (
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}
	
	// Parse parameters - we're now at the '(' token
	var parameters []*Parameter
	var restParameter *RestParameter
	var err error
	parameters, restParameter, err = p.parseFunctionParameters(true) // Allow parameter properties in constructors
	if err != nil {
		p.addError(p.curToken, fmt.Sprintf("failed to parse constructor parameters: %s", err.Error()))
		return nil
	}
	
	// Parse return type annotation if present
	var returnTypeAnnotation Expression
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // consume ':'
		p.nextToken() // move to start of type expression
		
		// Parse the return type using existing type parsing logic
		returnTypeAnnotation = p.parseTypeExpression()
		if returnTypeAnnotation == nil {
			return nil
		}
	}
	
	// Check if this is a constructor signature (ends with semicolon) or implementation (has body)
	if p.peekTokenIs(lexer.SEMICOLON) {
		// This is a constructor signature, not an implementation
		p.nextToken() // Consume semicolon
		
		sig := &ConstructorSignature{
			Token:                constructorToken,
			TypeParameters:       typeParameters,
			Parameters:           parameters,
			RestParameter:        restParameter,
			ReturnTypeAnnotation: returnTypeAnnotation,
			IsStatic:             isStatic,
			IsPublic:             isPublic,
			IsPrivate:            isPrivate,
			IsProtected:          isProtected,
		}
		return sig
	}
	
	// This is a constructor implementation - continue parsing the body
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}
	
	body := p.parseBlockStatement()
	
	// parseBlockStatement leaves us at '}', advance past it
	p.nextToken()
	
	// Create function literal for the implementation
	functionLiteral := &FunctionLiteral{
		Token:                constructorToken,
		Name:                 nil, // Constructors don't have names like regular functions
		TypeParameters:       typeParameters,
		Parameters:           parameters,
		RestParameter:        restParameter,
		ReturnTypeAnnotation: returnTypeAnnotation,
		Body:                 body,
	}
	
	return &MethodDefinition{
		Token:       constructorToken,
		Key:         &Identifier{Token: constructorToken, Value: "constructor"},
		Value:       functionLiteral,
		Kind:        "constructor",
		IsStatic:    isStatic,
		IsPublic:    isPublic,
		IsPrivate:   isPrivate,
		IsProtected: isProtected,
	}
}

// parseMethod parses a regular method or signature
// Returns either *MethodSignature or *MethodDefinition based on whether it ends with ';' or '{'
func (p *Parser) parseMethod(isStatic, isPublic, isPrivate, isProtected, isAbstract, isOverride bool) interface{} {
	methodToken := p.curToken
	methodName := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	
	// Try to parse type parameters: methodName<T, U>()
	typeParameters := p.tryParseTypeParameters()
	
	// We're currently at the method name token (or after type params), next should be (
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}
	
	// Parse parameters - we're now at the '(' token
	var parameters []*Parameter
	var restParameter *RestParameter
	var err error
	parameters, restParameter, err = p.parseFunctionParameters(false) // No parameter properties in regular methods
	if err != nil {
		p.addError(p.curToken, fmt.Sprintf("failed to parse method parameters: %s", err.Error()))
		return nil
	}
	
	// Parse return type annotation if present
	var returnTypeAnnotation Expression
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // consume ':'
		p.nextToken() // move to start of type expression
		
		// Parse the return type using existing type parsing logic
		returnTypeAnnotation = p.parseTypeExpression()
		if returnTypeAnnotation == nil {
			return nil
		}
	}
	
	// Check if this is a method signature (ends with semicolon) or implementation (has body)
	// Abstract methods must be signatures (no implementation)
	if p.peekTokenIs(lexer.SEMICOLON) || isAbstract {
		// This is a method signature, not an implementation
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken() // Consume semicolon
		}
		
		sig := &MethodSignature{
			Token:                methodToken,
			Key:                  methodName,
			TypeParameters:       typeParameters,
			Parameters:           parameters,
			RestParameter:        restParameter,
			ReturnTypeAnnotation: returnTypeAnnotation,
			Kind:                 "method",
			IsStatic:             isStatic,
			IsPublic:             isPublic,
			IsPrivate:            isPrivate,
			IsProtected:          isProtected,
			IsAbstract:           isAbstract,
			IsOverride:           isOverride,
		}
		return sig
	}
	
	// Abstract methods cannot have implementations
	if isAbstract {
		p.addError(p.curToken, "abstract methods cannot have implementations")
		return nil
	}
	
	// This is a method implementation - continue parsing the body
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}
	
	body := p.parseBlockStatement()
	
	// parseBlockStatement leaves us at '}', advance past it
	p.nextToken()
	
	// Create function literal for the implementation
	functionLiteral := &FunctionLiteral{
		Token:                methodToken,
		Name:                 nil, // Methods don't have names in the traditional function sense
		TypeParameters:       typeParameters,
		Parameters:           parameters,
		RestParameter:        restParameter,
		ReturnTypeAnnotation: returnTypeAnnotation,
		Body:                 body,
	}
	
	return &MethodDefinition{
		Token:       methodToken,
		Key:         methodName,
		Value:       functionLiteral,
		Kind:        "method",
		IsStatic:    isStatic,
		IsPublic:    isPublic,
		IsPrivate:   isPrivate,
		IsProtected: isProtected,
		IsOverride:  isOverride,
	}
}

// parseProperty parses a property declaration
func (p *Parser) parseProperty(isStatic, isReadonly, isPublic, isPrivate, isProtected bool) *PropertyDefinition {
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
		Readonly:       isReadonly,
		IsPublic:       isPublic,
		IsPrivate:      isPrivate,
		IsProtected:    isProtected,
	}
}

// parseGetter parses a getter method in a class
// Syntax: [static] get propertyName(): returnType { body }
func (p *Parser) parseGetter(isStatic, isPublic, isPrivate, isProtected, isOverride bool) *MethodDefinition {
	getToken := p.curToken // 'get' token
	
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	
	propertyName := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}
	
	// Use existing function parameter parsing infrastructure
	functionLiteral := &FunctionLiteral{
		Token: getToken,
		Name:  nil,
	}
	
	// Parse parameters - we're now at the '(' token
	var err error
	functionLiteral.Parameters, functionLiteral.RestParameter, err = p.parseFunctionParameters(false) // No parameter properties in getters
	if err != nil {
		p.addError(p.curToken, fmt.Sprintf("failed to parse getter parameters: %s", err.Error()))
		return nil
	}
	
	// Validate that getters have no parameters
	if len(functionLiteral.Parameters) > 0 || functionLiteral.RestParameter != nil {
		p.addError(p.curToken, "getters cannot have parameters")
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
		Token:       getToken,
		Key:         propertyName,
		Value:       functionLiteral,
		Kind:        "getter",
		IsStatic:    isStatic,
		IsPublic:    isPublic,
		IsPrivate:   isPrivate,
		IsProtected: isProtected,
		IsOverride:  isOverride,
	}
}

// parseSetter parses a setter method in a class
// Syntax: [static] set propertyName(value: type) { body }
func (p *Parser) parseSetter(isStatic, isPublic, isPrivate, isProtected, isOverride bool) *MethodDefinition {
	setToken := p.curToken // 'set' token
	
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	
	propertyName := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}
	
	// Use existing function parameter parsing infrastructure
	functionLiteral := &FunctionLiteral{
		Token: setToken,
		Name:  nil,
	}
	
	// Parse parameters - we're now at the '(' token
	var err error
	functionLiteral.Parameters, functionLiteral.RestParameter, err = p.parseFunctionParameters(false) // No parameter properties in setters
	if err != nil {
		p.addError(p.curToken, fmt.Sprintf("failed to parse setter parameters: %s", err.Error()))
		return nil
	}
	
	// Validate that setters have exactly one parameter
	if len(functionLiteral.Parameters) != 1 || functionLiteral.RestParameter != nil {
		p.addError(p.curToken, "setters must have exactly one parameter")
		return nil
	}
	
	// Parse body - after parseFunctionParameters we should be at ')' with peek being '{'
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}
	
	functionLiteral.Body = p.parseBlockStatement()
	
	// parseBlockStatement leaves us at '}', advance past it
	p.nextToken()
	
	return &MethodDefinition{
		Token:       setToken,
		Key:         propertyName,
		Value:       functionLiteral,
		Kind:        "setter",
		IsStatic:    isStatic,
		IsPublic:    isPublic,
		IsPrivate:   isPrivate,
		IsProtected: isProtected,
		IsOverride:  isOverride,
	}
}

// parseComputedClassMember parses computed properties and methods in class bodies
// Syntax: [expression]: type = value or [expression]() { ... }
func (p *Parser) parseComputedClassMember(isStatic, isReadonly, isPublic, isPrivate, isProtected, isAbstract, isOverride bool) interface{} {
	// We're currently at the '[' token
	if !p.curTokenIs(lexer.LBRACKET) {
		p.addError(p.curToken, "internal error: parseComputedClassMember called without '['")
		return nil
	}
	
	bracketToken := p.curToken
	
	// Parse the computed key expression
	p.nextToken() // move past '['
	keyExpr := p.parseExpression(LOWEST)
	if keyExpr == nil {
		p.addError(p.curToken, "expected expression inside computed property brackets")
		return nil
	}
	
	if !p.expectPeek(lexer.RBRACKET) {
		return nil // Missing closing ']'
	}
	
	// Now determine if this is a method or property based on what follows
	if p.peekTokenIs(lexer.LPAREN) || p.peekTokenIs(lexer.LT) {
		// It's a computed method: [expr]() {} or [expr]<T>() {}
		return p.parseComputedMethod(bracketToken, keyExpr, isStatic, isPublic, isPrivate, isProtected, isAbstract, isOverride)
	} else {
		// It's a computed property: [expr]: type = value
		return p.parseComputedProperty(bracketToken, keyExpr, isStatic, isReadonly, isPublic, isPrivate, isProtected)
	}
}

// parseComputedMethod parses a computed method in a class
func (p *Parser) parseComputedMethod(bracketToken lexer.Token, keyExpr Expression, isStatic, isPublic, isPrivate, isProtected, isAbstract, isOverride bool) interface{} {
	// Try to parse type parameters: [expr]<T, U>()
	typeParameters := p.tryParseTypeParameters()
	
	// We should be at the '(' token now
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}
	
	// Parse parameters - we're now at the '(' token
	var parameters []*Parameter
	var restParameter *RestParameter
	var err error
	parameters, restParameter, err = p.parseFunctionParameters(false) // No parameter properties in computed methods
	if err != nil {
		p.addError(p.curToken, fmt.Sprintf("failed to parse method parameters: %s", err.Error()))
		return nil
	}
	
	// Parse return type annotation if present
	var returnTypeAnnotation Expression
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // consume ':'
		p.nextToken() // move to start of type expression
		
		// Parse the return type using existing type parsing logic
		returnTypeAnnotation = p.parseTypeExpression()
		if returnTypeAnnotation == nil {
			return nil
		}
	}
	
	// Check if this is a method signature (ends with semicolon) or implementation (has body)
	// Abstract methods must be signatures (no implementation)
	if p.peekTokenIs(lexer.SEMICOLON) || isAbstract {
		// This is a method signature, not an implementation
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken() // Consume semicolon
		}
		
		sig := &MethodSignature{
			Token:                bracketToken,
			Key:                  &ComputedPropertyName{Expr: keyExpr}, // Use computed property name
			TypeParameters:       typeParameters,
			Parameters:           parameters,
			RestParameter:        restParameter,
			ReturnTypeAnnotation: returnTypeAnnotation,
			Kind:                 "method",
			IsStatic:             isStatic,
			IsPublic:             isPublic,
			IsPrivate:            isPrivate,
			IsProtected:          isProtected,
			IsAbstract:           isAbstract,
			IsOverride:           isOverride,
		}
		return sig
	}
	
	// Abstract methods cannot have implementations
	if isAbstract {
		p.addError(p.curToken, "abstract methods cannot have implementations")
		return nil
	}
	
	// This is a method implementation - continue parsing the body
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}
	
	body := p.parseBlockStatement()
	
	// parseBlockStatement leaves us at '}', advance past it
	p.nextToken()
	
	// Create function literal for the implementation
	functionLiteral := &FunctionLiteral{
		Token:                bracketToken,
		Name:                 nil, // Methods don't have names in the traditional function sense
		TypeParameters:       typeParameters,
		Parameters:           parameters,
		RestParameter:        restParameter,
		ReturnTypeAnnotation: returnTypeAnnotation,
		Body:                 body,
	}
	
	return &MethodDefinition{
		Token:       bracketToken,
		Key:         &ComputedPropertyName{Expr: keyExpr}, // Use computed property name
		Value:       functionLiteral,
		Kind:        "method",
		IsStatic:    isStatic,
		IsPublic:    isPublic,
		IsPrivate:   isPrivate,
		IsProtected: isProtected,
		IsOverride:  isOverride,
	}
}

// parseComputedProperty parses a computed property in a class
func (p *Parser) parseComputedProperty(bracketToken lexer.Token, keyExpr Expression, isStatic, isReadonly, isPublic, isPrivate, isProtected bool) *PropertyDefinition {
	// Check for optional marker '?' first
	var isOptional bool
	if p.peekTokenIs(lexer.QUESTION) {
		p.nextToken() // Consume '?'
		isOptional = true
	}
	
	// Parse optional type annotation using interface pattern
	var typeAnnotation Expression
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Move to ':'
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
		Token:          bracketToken,
		Key:            &ComputedPropertyName{Expr: keyExpr}, // Use computed property name
		TypeAnnotation: typeAnnotation,
		Value:          initializer,
		IsStatic:       isStatic,
		Optional:       isOptional,
		Readonly:       isReadonly,
		IsPublic:       isPublic,
		IsPrivate:      isPrivate,
		IsProtected:    isProtected,
	}
}

