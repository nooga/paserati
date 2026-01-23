package parser

import (
	"fmt"

	"github.com/nooga/paserati/pkg/lexer"
)

// isValidMethodName checks if the current token can be used as a method name
// In JavaScript/TypeScript, keywords and string literals can be used as method names
func (p *Parser) isValidMethodName() bool {
	switch p.curToken.Type {
	case lexer.IDENT:
		return true
	case lexer.STRING:
		return true
	case lexer.NUMBER: // Allow numeric method names like 42()
		return true
	case lexer.BIGINT: // Allow bigint method names like 1n()
		return true
	case lexer.PRIVATE_IDENT: // Allow private identifiers like #privateName()
		return true
	// Allow keywords as method names
	case lexer.RETURN, lexer.IF, lexer.ELSE, lexer.FOR, lexer.WHILE, lexer.FUNCTION,
		lexer.LET, lexer.CONST, lexer.VAR, lexer.CLASS, lexer.INTERFACE, lexer.TYPE,
		lexer.IMPORT, lexer.EXPORT, lexer.EXTENDS, lexer.IMPLEMENTS, lexer.ABSTRACT,
		lexer.STATIC, lexer.PUBLIC, lexer.PRIVATE, lexer.PROTECTED, lexer.READONLY,
		lexer.YIELD, lexer.TRY, lexer.CATCH, lexer.FINALLY, lexer.THROW, lexer.BREAK,
		lexer.CONTINUE, lexer.SWITCH, lexer.CASE, lexer.DEFAULT, lexer.NEW, lexer.DELETE,
		lexer.TYPEOF, lexer.INSTANCEOF, lexer.IN, lexer.OF, lexer.THIS, lexer.SUPER,
		lexer.AS, lexer.SATISFIES, lexer.GET, lexer.SET, lexer.ENUM, lexer.DO, lexer.VOID,
		lexer.KEYOF, lexer.INFER, lexer.IS, lexer.FROM, lexer.TRUE, lexer.FALSE,
		lexer.WITH, lexer.ASYNC, lexer.AWAIT, lexer.NULL, lexer.UNDEFINED:
		return true
	default:
		return false
	}
}

// parseClassDeclaration parses a class declaration statement
// Syntax: class ClassName [extends SuperClass] { classBody }
func (p *Parser) parseClassDeclaration() Statement {
	classToken := p.curToken

	// Class name can be an identifier or 'await'/'yield' in script mode (non-module)
	// These are the only contextual keywords valid as class names per ECMAScript spec
	if !p.peekTokenIs(lexer.IDENT) && !p.peekTokenIs(lexer.AWAIT) && !p.peekTokenIs(lexer.YIELD) {
		p.peekError(lexer.IDENT)
		return nil
	}
	p.nextToken()

	name := &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Parse type parameters if present (same pattern as interfaces)
	typeParameters := p.tryParseTypeParameters()

	var superClass Expression
	if p.peekTokenIs(lexer.EXTENDS) {
		p.nextToken() // consume 'extends'
		p.nextToken() // move to start of expression

		// Check if this is a runtime expression (function/class literal, call, parenthesized expr)
		// or a type expression (identifier with optional generic args)
		if p.curTokenIs(lexer.FUNCTION) || p.curTokenIs(lexer.CLASS) || p.curTokenIs(lexer.LPAREN) {
			// Runtime expression: function() {}, class {}, (expr), etc.
			superClass = p.parseExpression(LOWEST)
		} else {
			// Type expression: Container, Container<T>, etc.
			superClass = p.parseTypeExpression()
		}

		if superClass == nil {
			return nil // Failed to parse superclass expression
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
	// Class name can be an identifier or 'await'/'yield' in script mode (non-module)
	// These are the only contextual keywords valid as class names per ECMAScript spec
	if p.peekTokenIs(lexer.IDENT) || p.peekTokenIs(lexer.AWAIT) || p.peekTokenIs(lexer.YIELD) {
		p.nextToken()
		name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

	// Parse type parameters if present (same pattern as interfaces)
	// Note: Anonymous classes (name == nil) can still have type parameters
	typeParameters := p.tryParseTypeParameters()

	var superClass Expression
	if p.peekTokenIs(lexer.EXTENDS) {
		p.nextToken() // consume 'extends'
		p.nextToken() // move to start of expression

		// Check if this is a runtime expression (function/class literal, call, parenthesized expr)
		// or a type expression (identifier with optional generic args)
		if p.curTokenIs(lexer.FUNCTION) || p.curTokenIs(lexer.CLASS) || p.curTokenIs(lexer.LPAREN) {
			// Runtime expression: function() {}, class {}, (expr), etc.
			superClass = p.parseExpression(LOWEST)
		} else {
			// Type expression: Container, Container<T>, etc.
			superClass = p.parseTypeExpression()
		}

		if superClass == nil {
			return nil // Failed to parse superclass expression
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
	var staticInitializers []*BlockStatement

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
		isAsync := false

		// Helper to check if current token is likely a field name (not a modifier)
		// A keyword is a field name if followed by ;, =, :, or ?
		isFieldName := func() bool {
			return p.peekTokenIs(lexer.SEMICOLON) || p.peekTokenIs(lexer.ASSIGN) ||
				p.peekTokenIs(lexer.COLON) || p.peekTokenIs(lexer.QUESTION)
		}

		// Check for static initializer block before parsing modifiers: static { ... }
		if p.curTokenIs(lexer.STATIC) && p.peekTokenIs(lexer.LBRACE) {
			p.nextToken() // move to '{'
			block := p.parseBlockStatement()
			if block != nil {
				staticInitializers = append(staticInitializers, block)
			}
			p.nextToken() // move past '}'
			continue      // Done with this class member, continue to next
		}

		// Parse all modifiers until we hit something that's not a modifier
		for {
			if p.curTokenIs(lexer.READONLY) && !isReadonly && !isFieldName() {
				isReadonly = true
				p.nextToken()
			} else if p.curTokenIs(lexer.STATIC) && !isStatic && !isFieldName() {
				isStatic = true
				p.nextToken()
			} else if p.curTokenIs(lexer.PUBLIC) && !isPublic && !isPrivate && !isProtected && !isFieldName() {
				isPublic = true
				p.nextToken()
			} else if p.curTokenIs(lexer.PRIVATE) && !isPublic && !isPrivate && !isProtected && !isFieldName() {
				isPrivate = true
				p.nextToken()
			} else if p.curTokenIs(lexer.PROTECTED) && !isPublic && !isPrivate && !isProtected && !isFieldName() {
				isProtected = true
				p.nextToken()
			} else if p.curTokenIs(lexer.ABSTRACT) && !isAbstract && !isFieldName() {
				isAbstract = true
				p.nextToken()
			} else if p.curTokenIs(lexer.OVERRIDE) && !isOverride && !isFieldName() {
				isOverride = true
				p.nextToken()
			} else if p.curTokenIs(lexer.ASYNC) && !isAsync && !isFieldName() {
				isAsync = true
				p.nextToken()
			} else {
				break // No more modifiers
			}
		}

		// Parse constructor, method, getter, setter, generator, or computed member
		// Per ECMAScript spec: "get [no LineTerminator here] ClassElementName"
		// If there's a newline after 'get', it's a field named "get", not a getter accessor
		hasNewlineAfterCur := p.peekToken.Line > p.curToken.Line
		if p.curTokenIs(lexer.GET) && !p.peekTokenIs(lexer.LPAREN) && !hasNewlineAfterCur {
			// Parse getter method: get propertyName() {}
			// But NOT if followed by '(' - that's a method named "get": get() {}
			// And NOT if there's a newline (ASI) - that's a field named "get"
			method := p.parseGetter(isStatic, isPublic, isPrivate, isProtected, isOverride)
			if method != nil {
				methods = append(methods, method)
			}
		} else if p.curTokenIs(lexer.SET) && !p.peekTokenIs(lexer.LPAREN) && !hasNewlineAfterCur {
			// Parse setter method: set propertyName(value) {}
			// But NOT if followed by '(' - that's a method named "set": set() {}
			// And NOT if there's a newline (ASI) - that's a field named "set"
			method := p.parseSetter(isStatic, isPublic, isPrivate, isProtected, isOverride)
			if method != nil {
				methods = append(methods, method)
			}
		} else if p.curTokenIs(lexer.ASTERISK) {
			// Parse generator method: *methodName() { ... }
			// Could be async generator if isAsync is true
			method := p.parseGeneratorMethod(isStatic, isPublic, isPrivate, isProtected, isAbstract, isOverride)
			if method != nil {
				// Mark as async if we saw async keyword
				if isAsync && method.Value != nil {
					method.Value.IsAsync = true
				}
				methods = append(methods, method)
			}
		} else if isAsync {
			// async method (without asterisk): async methodName() { ... }
			method := p.parseAsyncMethod(isStatic, isPublic, isPrivate, isProtected, isAbstract, isOverride)
			if method != nil {
				methods = append(methods, method)
			}
		} else if p.curTokenIs(lexer.LBRACKET) {
			// Computed property or method: [expr]: type or [expr]() {} or [expr] = value
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
		} else if p.curTokenIs(lexer.PRIVATE_IDENT) {
			// IMPORTANT: Check for private fields/methods (#name) BEFORE isValidMethodName()
			// because isValidMethodName() also matches PRIVATE_IDENT tokens
			// Private field or method: #fieldName or #methodName()
			// Private fields/methods are always implicitly private
			// They cannot have explicit access modifiers
			if isPublic || isPrivate || isProtected {
				p.addError(p.curToken, "private fields/methods (#name) cannot have explicit access modifiers")
				p.nextToken()
				continue
			}

			// Check if this is a private method (followed by '(') or private field
			if p.peekTokenIs(lexer.LPAREN) {
				// Private method: #methodName()
				method := p.parsePrivateMethod(isStatic)
				if method != nil {
					methods = append(methods, method)
				}
			} else {
				// Private field: #fieldName
				property := p.parsePrivateProperty(isStatic, isReadonly)
				if property != nil {
					properties = append(properties, property)
				}
			}
		} else if p.isValidMethodName() {
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
		} else {
			// Remove duplicate PRIVATE_IDENT check - now handled earlier in the chain
			debugPrint("ERROR: Unrecognized token in class body: cur='%s' (%s), peek='%s' (%s)", p.curToken.Literal, p.curToken.Type, p.peekToken.Literal, p.peekToken.Type)
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
		Token:              bodyToken,
		Methods:            methods,
		Properties:         properties,
		ConstructorSigs:    constructorSigs,
		MethodSigs:         methodSigs,
		StaticInitializers: staticInitializers,
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

	// Transform destructuring parameters
	functionLiteral = p.transformFunctionWithDestructuring(functionLiteral)

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
	var methodName Expression
	if p.curToken.Type == lexer.STRING {
		// String literal method name: "methodName"()
		methodName = &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
	} else if p.curToken.Type == lexer.NUMBER {
		// Numeric literal method name: 42()
		methodName = p.parseNumberLiteral()
	} else if p.curToken.Type == lexer.BIGINT {
		// Bigint literal method name: 1n()
		methodName = p.parseBigIntLiteral()
	} else {
		// Identifier method name
		methodName = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

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

	// Transform destructuring parameters
	functionLiteral = p.transformFunctionWithDestructuring(functionLiteral)

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

// parseAsyncMethod parses an async method in a class
// Reuses parseMethod logic but marks the result as async
func (p *Parser) parseAsyncMethod(isStatic, isPublic, isPrivate, isProtected, isAbstract, isOverride bool) *MethodDefinition {
	result := p.parseMethod(isStatic, isPublic, isPrivate, isProtected, isAbstract, isOverride)
	if result == nil {
		return nil
	}

	// parseMethod can return either MethodSignature or MethodDefinition
	if method, ok := result.(*MethodDefinition); ok {
		// Mark the function literal as async
		if method.Value != nil {
			method.Value.IsAsync = true
		}
		return method
	}

	// If it's a signature, we can't make it async (would need to return MethodSignature)
	// For now, just treat as method definition
	return nil
}

// parseProperty parses a property declaration
func (p *Parser) parseProperty(isStatic, isReadonly, isPublic, isPrivate, isProtected bool) *PropertyDefinition {
	propertyToken := p.curToken
	var propertyName Expression
	if p.curToken.Type == lexer.STRING {
		// String literal property name: "propertyName"
		propertyName = &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
	} else {
		// Identifier property name
		propertyName = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

	// At this point: cur=fieldName, peek=next token
	// We need to check what comes after the field name

	// Check for optional marker '?' in peek position
	var isOptional bool
	if p.peekTokenIs(lexer.QUESTION) {
		p.nextToken() // Move to '?'
		isOptional = true
	}

	// Now we're either still at fieldName or at '?'
	// Advance past it
	p.nextToken()

	// Parse optional type annotation
	var typeAnnotation Expression
	if p.curTokenIs(lexer.COLON) {
		p.nextToken() // Move past ':'

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
		// Use COMMA precedence (lower than ASSIGNMENT) to allow assignment operators to be parsed
		// This allows chained assignments like: x = obj['lol'] = 42
		initializer = p.parseExpression(COMMA)

		// After parseExpression, curToken is at the last token of the expression.
		// We need to advance to the next token for the class body parser.
		// Handle ASI: if there's a newline before the next token and no explicit semicolon,
		// treat it as if there was a semicolon (Automatic Semicolon Insertion).
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken() // Move to semicolon
			p.nextToken() // Move past semicolon to next class member
		} else {
			// ASI case: no explicit semicolon, but we still need to advance
			// past the last token of the expression to the next class member
			p.nextToken()
		}
	} else {
		// No initializer
		// Check for optional semicolon and skip it
		if p.curTokenIs(lexer.SEMICOLON) {
			p.nextToken() // Move past semicolon
		}
		// Now curToken should be at the next member or '}'
		// No additional advancement needed
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

// parsePrivateProperty parses a private field declaration (#fieldName)
// Private fields are implicitly private and cannot have explicit access modifiers
func (p *Parser) parsePrivateProperty(isStatic, isReadonly bool) *PropertyDefinition {
	propertyToken := p.curToken
	// Create identifier from PRIVATE_IDENT token (includes the '#')
	propertyName := &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check for optional marker '?' in peek position
	var isOptional bool
	if p.peekTokenIs(lexer.QUESTION) {
		p.nextToken() // Move to '?'
		isOptional = true
	}

	// Advance past field name or '?'
	p.nextToken()

	// Parse optional type annotation
	var typeAnnotation Expression
	if p.curTokenIs(lexer.COLON) {
		p.nextToken() // Move past ':'

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
		// Use COMMA precedence (lower than ASSIGNMENT) to allow assignment operators to be parsed
		// This allows chained assignments like: x = obj['lol'] = 42
		initializer = p.parseExpression(COMMA)

		// After parseExpression, curToken is at the last token of the expression.
		// We need to advance to the next token for the class body parser.
		// Handle ASI: if there's a newline before the next token and no explicit semicolon,
		// treat it as if there was a semicolon (Automatic Semicolon Insertion).
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken() // Move to semicolon
			p.nextToken() // Move past semicolon to next class member
		} else {
			// ASI case: no explicit semicolon, but we still need to advance
			// past the last token of the expression to the next class member
			p.nextToken()
		}
	} else {
		// No initializer
		// Check for optional semicolon and skip it
		if p.curTokenIs(lexer.SEMICOLON) {
			p.nextToken() // Move past semicolon
		}
		// Now curToken should be at the next member or '}'
		// No additional advancement needed
	}

	return &PropertyDefinition{
		Token:          propertyToken,
		Key:            propertyName,
		TypeAnnotation: typeAnnotation,
		Value:          initializer,
		IsStatic:       isStatic,
		Optional:       isOptional,
		Readonly:       isReadonly,
		IsPublic:       false, // Private fields are never public
		IsPrivate:      true,  // Private fields are always private
		IsProtected:    false, // Private fields are never protected
	}
}

// parseGetter parses a getter method in a class
// Syntax: [static] get propertyName(): returnType { body } or [static] get [computed](): returnType { body }
func (p *Parser) parseGetter(isStatic, isPublic, isPrivate, isProtected, isOverride bool) *MethodDefinition {
	getToken := p.curToken // 'get' token

	// Check if next token is an identifier, string literal, number, private identifier, or computed property
	if !p.peekTokenIs(lexer.IDENT) && !p.peekTokenIs(lexer.STRING) && !p.peekTokenIs(lexer.NUMBER) && !p.peekTokenIs(lexer.PRIVATE_IDENT) && !p.peekTokenIs(lexer.LBRACKET) {
		p.addError(p.peekToken, "expected identifier, string literal, number, private identifier, or computed property after 'get'")
		// Advance past the problematic token to avoid infinite loop
		p.nextToken()
		return nil
	}
	p.nextToken() // Move to property name token

	var propertyName Expression
	if p.curToken.Type == lexer.STRING {
		// String literal property name: get "propertyName"()
		propertyName = &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
	} else if p.curToken.Type == lexer.NUMBER {
		// Numeric literal property name: get 0x10() or get 42()
		// Use the existing number parsing logic from parseNumberLiteral
		numLit := p.parseNumberLiteral().(*NumberLiteral)
		propertyName = numLit
	} else if p.curToken.Type == lexer.LBRACKET {
		// Computed property name: get [expr]()
		p.nextToken() // move past '['
		keyExpr := p.parseExpression(COMMA)
		if keyExpr == nil {
			p.addError(p.curToken, "expected expression inside computed property brackets")
			return nil
		}
		if !p.expectPeek(lexer.RBRACKET) {
			return nil // Missing closing ']'
		}
		propertyName = &ComputedPropertyName{Expr: keyExpr}
	} else if p.curToken.Type == lexer.PRIVATE_IDENT {
		// Private identifier property name: get #privateName()
		propertyName = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	} else {
		// Regular identifier property name
		propertyName = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

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
// Syntax: [static] set propertyName(value: type) { body } or [static] set [computed](value: type) { body }
func (p *Parser) parseSetter(isStatic, isPublic, isPrivate, isProtected, isOverride bool) *MethodDefinition {
	setToken := p.curToken // 'set' token

	// Check if next token is an identifier, string literal, number, private identifier, or computed property
	if !p.peekTokenIs(lexer.IDENT) && !p.peekTokenIs(lexer.STRING) && !p.peekTokenIs(lexer.NUMBER) && !p.peekTokenIs(lexer.PRIVATE_IDENT) && !p.peekTokenIs(lexer.LBRACKET) {
		p.addError(p.peekToken, "expected identifier, string literal, number, private identifier, or computed property after 'set'")
		// Advance past the problematic token to avoid infinite loop
		p.nextToken()
		return nil
	}
	p.nextToken() // Move to property name token

	var propertyName Expression
	if p.curToken.Type == lexer.STRING {
		// String literal property name: set "propertyName"()
		propertyName = &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
	} else if p.curToken.Type == lexer.NUMBER {
		// Numeric literal property name: set 0x10() or set 42()
		// Use the existing number parsing logic from parseNumberLiteral
		numLit := p.parseNumberLiteral().(*NumberLiteral)
		propertyName = numLit
	} else if p.curToken.Type == lexer.LBRACKET {
		// Computed property name: set [expr]()
		p.nextToken() // move past '['
		keyExpr := p.parseExpression(COMMA)
		if keyExpr == nil {
			p.addError(p.curToken, "expected expression inside computed property brackets")
			return nil
		}
		if !p.expectPeek(lexer.RBRACKET) {
			return nil // Missing closing ']'
		}
		propertyName = &ComputedPropertyName{Expr: keyExpr}
	} else if p.curToken.Type == lexer.PRIVATE_IDENT {
		// Private identifier property name: set #privateName()
		propertyName = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	} else {
		// Regular identifier property name
		propertyName = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

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

	// Transform destructuring parameters
	functionLiteral = p.transformFunctionWithDestructuring(functionLiteral)

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
	keyExpr := p.parseExpression(COMMA)
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
		// It's a computed property: [expr]: type = value or [expr] = value
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

	// Transform destructuring parameters
	functionLiteral = p.transformFunctionWithDestructuring(functionLiteral)

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
	}

	var initializer Expression
	if p.peekTokenIs(lexer.ASSIGN) {
		p.nextToken() // Move to '='
		p.nextToken() // Move past '=' to the initializer expression
		// Use COMMA precedence (lower than ASSIGNMENT) to allow assignment operators to be parsed
		// This allows chained assignments like: x = obj['lol'] = 42
		initializer = p.parseExpression(COMMA)

		// Consume optional semicolon after initializer
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken() // Move to semicolon
		}

		// Advance past the current token to prepare for the next class member
		// parseExpression leaves us AT the last token, so we need to advance
		if !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
			p.nextToken()
		}
	} else {
		// No initializer - we're still at the ']' token from parseComputedClassMember
		// We need to advance to the next token
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken() // Move to semicolon
		}
		// Advance past current token (either ']' or ';') to next member
		if !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
			p.nextToken()
		}
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

// parseGeneratorMethod parses a generator method in a class
// Syntax: [static] *methodName() { body }
func (p *Parser) parseGeneratorMethod(isStatic, isPublic, isPrivate, isProtected, isAbstract, isOverride bool) *MethodDefinition {
	asteriskToken := p.curToken // '*' token

	// Move past '*' to get method name
	p.nextToken() // Consume '*'

	// Check that we have a valid method name token
	// Use isValidMethodName() to allow keywords like 'yield', 'await', etc. as method names
	if !p.isValidMethodName() && !p.curTokenIs(lexer.LBRACKET) {
		p.addError(p.curToken, "expected method name after '*' in generator method")
		return nil
	}

	var methodName Expression
	var methodToken lexer.Token

	if p.curTokenIs(lexer.LBRACKET) {
		// Computed generator method: *[expr]() { ... }
		methodToken = p.curToken
		p.nextToken() // Consume '['
		keyExpr := p.parseExpression(COMMA)
		if keyExpr == nil {
			return nil
		}
		if !p.expectPeek(lexer.RBRACKET) {
			return nil
		}
		methodName = &ComputedPropertyName{Expr: keyExpr}
	} else if p.curTokenIs(lexer.STRING) {
		// String literal generator method: *"methodName"() { ... }
		methodToken = p.curToken
		methodName = &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
	} else if p.curTokenIs(lexer.PRIVATE_IDENT) {
		// Private generator method: *#methodName() { ... }
		methodToken = p.curToken
		methodName = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	} else {
		// Regular identifier generator method: *methodName() { ... }
		methodToken = p.curToken
		methodName = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

	// Try to parse type parameters: *methodName<T, U>()
	typeParameters := p.tryParseTypeParameters()

	// Expect '(' for parameters
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	// Parse parameters
	var parameters []*Parameter
	var restParameter *RestParameter
	var err error
	parameters, restParameter, err = p.parseFunctionParameters(false) // No parameter properties in generator methods
	if err != nil {
		p.addError(p.curToken, fmt.Sprintf("failed to parse generator method parameters: %s", err.Error()))
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

	// Abstract generator methods cannot have implementations
	if isAbstract {
		p.addError(p.curToken, "abstract generator methods cannot have implementations")
		return nil
	}

	// Expect '{' for method body
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	// Increment generator context before parsing body
	p.inGenerator++
	if debugParser {
		fmt.Printf("[PARSER] Entering generator context (class generator method), inGenerator=%d\n", p.inGenerator)
	}

	body := p.parseBlockStatement()

	// Restore generator context
	p.inGenerator--
	if debugParser {
		fmt.Printf("[PARSER] Leaving generator context (class generator method), inGenerator=%d\n", p.inGenerator)
	}

	// parseBlockStatement leaves us at '}', advance past it
	p.nextToken()

	// Create function literal for the implementation with IsGenerator = true
	functionLiteral := &FunctionLiteral{
		Token:                asteriskToken, // Use the '*' token
		Name:                 nil,           // Methods don't have names in the traditional function sense
		IsGenerator:          true,          // This is a generator function
		TypeParameters:       typeParameters,
		Parameters:           parameters,
		RestParameter:        restParameter,
		ReturnTypeAnnotation: returnTypeAnnotation,
		Body:                 body,
	}

	// Transform destructuring parameters
	functionLiteral = p.transformFunctionWithDestructuring(functionLiteral)

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
