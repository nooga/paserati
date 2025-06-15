package parser

import (
	"fmt"
	"paserati/pkg/errors"
	"paserati/pkg/lexer"
	"strconv"
	"strings"
	"unsafe"
)

// --- Debug Flag ---
const debugParser = false

func debugPrint(format string, args ...interface{}) {
	if debugParser {
		fmt.Printf("[Parser Debug] "+format+"\n", args...)
	}
}

// --- End Debug Flag ---

// Parser takes a lexer and builds an AST.
type Parser struct {
	l      *lexer.Lexer
	errors []errors.PaseratiError

	curToken  lexer.Token
	peekToken lexer.Token

	// Pratt parser for VALUE expressions
	prefixParseFns map[lexer.TokenType]prefixParseFn
	infixParseFns  map[lexer.TokenType]infixParseFn

	// --- NEW: Pratt parser for TYPE expressions ---
	typePrefixParseFns map[lexer.TokenType]prefixParseFn // Handles starts of types (e.g., number, string, ident, (), [])
	typeInfixParseFns  map[lexer.TokenType]infixParseFn  // Handles type operators (e.g., |, &)
}

// Parsing functions types for Pratt parser
type (
	prefixParseFn func() Expression
	infixParseFn  func(Expression) Expression // Arg is the left side expression
)

// Precedence levels for VALUE operators
const (
	_ int = iota
	LOWEST
	ASSIGNMENT  // =, +=, -=, *=, /=, %=, **=, &=, |=, ^=, <<=, >>=, >>>=, &&=, ||=, ??=
	TERNARY     // ?:
	COALESCE    // ??
	LOGICAL_OR  // ||
	LOGICAL_AND // &&
	BITWISE_OR  // |  (Lower than XOR)
	BITWISE_XOR // ^  (Lower than AND)
	BITWISE_AND // &  (Lower than Equality)
	EQUALS      // ==, !=, ===, !==
	LESSGREATER // >, <, >=, <=
	SHIFT       // <<, >>, >>> (Lower than Add/Sub)
	SUM         // + or -
	PRODUCT     // * or / or %
	POWER       // ** (Right-associative handled in parseInfix)
	PREFIX      // -X or !X or ++X or --X or ~X
	POSTFIX     // X++ or X--
	ASSERTION   // value as Type
	CALL        // myFunction(X)
	INDEX       // array[index]
	MEMBER      // object.property
)

// --- NEW: Type Precedence ---
const (
	_ int = iota
	TYPE_LOWEST
	TYPE_UNION        // |
	TYPE_INTERSECTION // &  (Higher precedence than union)
	TYPE_ARRAY        // [] (Higher precedence than intersection)
)

// Precedences map for VALUE operator tokens
var precedences = map[lexer.TokenType]int{
	// Assignment (Lowest operational precedence)
	lexer.ASSIGN:                      ASSIGNMENT,
	lexer.PLUS_ASSIGN:                 ASSIGNMENT,
	lexer.MINUS_ASSIGN:                ASSIGNMENT,
	lexer.ASTERISK_ASSIGN:             ASSIGNMENT,
	lexer.SLASH_ASSIGN:                ASSIGNMENT,
	lexer.REMAINDER_ASSIGN:            ASSIGNMENT,
	lexer.EXPONENT_ASSIGN:             ASSIGNMENT,
	lexer.BITWISE_AND_ASSIGN:          ASSIGNMENT, // New
	lexer.BITWISE_OR_ASSIGN:           ASSIGNMENT, // New
	lexer.BITWISE_XOR_ASSIGN:          ASSIGNMENT, // New
	lexer.LEFT_SHIFT_ASSIGN:           ASSIGNMENT, // New
	lexer.RIGHT_SHIFT_ASSIGN:          ASSIGNMENT, // New
	lexer.UNSIGNED_RIGHT_SHIFT_ASSIGN: ASSIGNMENT, // New
	lexer.LOGICAL_AND_ASSIGN:          ASSIGNMENT, // New
	lexer.LOGICAL_OR_ASSIGN:           ASSIGNMENT, // New
	lexer.COALESCE_ASSIGN:             ASSIGNMENT, // New

	// Ternary, Logical, Coalescing
	lexer.QUESTION:    TERNARY,
	lexer.COALESCE:    COALESCE,
	lexer.LOGICAL_OR:  LOGICAL_OR,
	lexer.LOGICAL_AND: LOGICAL_AND,

	// Bitwise (Order: | < ^ < &)
	lexer.PIPE:        BITWISE_OR,  // Treat type union | at same level as bitwise | for now
	lexer.BITWISE_XOR: BITWISE_XOR, // New
	lexer.BITWISE_AND: BITWISE_AND, // New

	// Equality
	lexer.EQ:            EQUALS,
	lexer.NOT_EQ:        EQUALS,
	lexer.STRICT_EQ:     EQUALS,
	lexer.STRICT_NOT_EQ: EQUALS,

	// Comparison
	lexer.LT:         LESSGREATER,
	lexer.GT:         LESSGREATER,
	lexer.LE:         LESSGREATER,
	lexer.GE:         LESSGREATER,
	lexer.IN:         LESSGREATER,
	lexer.INSTANCEOF: LESSGREATER,

	// Shift
	lexer.LEFT_SHIFT:           SHIFT, // New
	lexer.RIGHT_SHIFT:          SHIFT, // New
	lexer.UNSIGNED_RIGHT_SHIFT: SHIFT, // New

	// Arithmetic
	lexer.PLUS:      SUM,
	lexer.MINUS:     SUM,
	lexer.SLASH:     PRODUCT,
	lexer.ASTERISK:  PRODUCT,
	lexer.REMAINDER: PRODUCT, // Existing
	lexer.EXPONENT:  POWER,   // Existing (Right-associative handled in infix parsing)

	// Prefix/Postfix (Handled by registration, not just precedence map)
	// lexer.BANG does not need precedence here (uses PREFIX in parsePrefix)
	// lexer.BITWISE_NOT does not need precedence here (uses PREFIX in parsePrefix)
	// lexer.INC prefix/postfix handled by registration
	// lexer.DEC prefix/postfix handled by registration

	// Type Assertion
	lexer.AS: ASSERTION,

	// Call, Index, Member Access
	lexer.LPAREN:            CALL,
	lexer.LBRACKET:          INDEX,
	lexer.DOT:               MEMBER,
	lexer.OPTIONAL_CHAINING: MEMBER, // Same precedence as regular member access

	// Postfix operators need precedence for the parseExpression loop termination condition
	lexer.INC: POSTFIX,
	lexer.DEC: POSTFIX,
}

// --- NEW: Precedences map for TYPE operator tokens ---
var typePrecedences = map[lexer.TokenType]int{
	lexer.PIPE:        TYPE_UNION,
	lexer.BITWISE_AND: TYPE_INTERSECTION,
	lexer.LBRACKET:    TYPE_ARRAY,
}

// NewParser creates a new Parser.
func NewParser(l *lexer.Lexer) *Parser {
	p := &Parser{
		l:      l,
		errors: []errors.PaseratiError{},
	}

	// Initialize Pratt parser maps for VALUE expressions
	p.prefixParseFns = make(map[lexer.TokenType]prefixParseFn)
	p.infixParseFns = make(map[lexer.TokenType]infixParseFn)

	// --- NEW: Initialize Pratt parser maps for TYPE expressions ---
	p.typePrefixParseFns = make(map[lexer.TokenType]prefixParseFn)
	p.typeInfixParseFns = make(map[lexer.TokenType]infixParseFn)

	// --- Register VALUE Prefix Functions ---
	p.registerPrefix(lexer.IDENT, p.parseIdentifier)
	p.registerPrefix(lexer.NUMBER, p.parseNumberLiteral)
	p.registerPrefix(lexer.STRING, p.parseStringLiteral)
	p.registerPrefix(lexer.TEMPLATE_START, p.parseTemplateLiteral) // NEW: Template literals
	p.registerPrefix(lexer.TRUE, p.parseBooleanLiteral)
	p.registerPrefix(lexer.FALSE, p.parseBooleanLiteral)
	p.registerPrefix(lexer.NULL, p.parseNullLiteral)
	p.registerPrefix(lexer.UNDEFINED, p.parseUndefinedLiteral) // Keep for value context
	p.registerPrefix(lexer.THIS, p.parseThisExpression)        // Added for this keyword
	p.registerPrefix(lexer.NEW, p.parseNewExpression)          // Added for new keyword
	p.registerPrefix(lexer.FUNCTION, p.parseFunctionLiteral)
	p.registerPrefix(lexer.BANG, p.parsePrefixExpression)
	p.registerPrefix(lexer.MINUS, p.parsePrefixExpression)
	p.registerPrefix(lexer.PLUS, p.parsePrefixExpression) // Added for unary plus
	p.registerPrefix(lexer.BITWISE_NOT, p.parsePrefixExpression)
	p.registerPrefix(lexer.TYPEOF, p.parseTypeofExpression) // Added for typeof operator
	p.registerPrefix(lexer.VOID, p.parseVoidExpression)     // Added for void operator
	p.registerPrefix(lexer.DELETE, p.parsePrefixExpression) // Added for delete operator
	p.registerPrefix(lexer.INC, p.parsePrefixUpdateExpression)
	p.registerPrefix(lexer.DEC, p.parsePrefixUpdateExpression)
	p.registerPrefix(lexer.LPAREN, p.parseGroupedExpression)
	p.registerPrefix(lexer.IF, p.parseIfExpression)
	p.registerPrefix(lexer.LBRACKET, p.parseArrayLiteral) // Value context: Array literal
	p.registerPrefix(lexer.LBRACE, p.parseObjectLiteral)  // <<< NEW: Register Object Literal Parsing
	p.registerPrefix(lexer.SPREAD, p.parseSpreadElement)  // NEW: Spread syntax in calls

	// --- Register VALUE Infix Functions ---
	// Arithmetic & Comparison/Logical
	p.registerInfix(lexer.PLUS, p.parseInfixExpression)
	p.registerInfix(lexer.MINUS, p.parseInfixExpression)
	p.registerInfix(lexer.SLASH, p.parseInfixExpression)
	p.registerInfix(lexer.ASTERISK, p.parseInfixExpression)
	p.registerInfix(lexer.REMAINDER, p.parseInfixExpression)
	p.registerInfix(lexer.EXPONENT, p.parseInfixExpression)
	p.registerInfix(lexer.EQ, p.parseInfixExpression)
	p.registerInfix(lexer.NOT_EQ, p.parseInfixExpression)
	p.registerInfix(lexer.STRICT_EQ, p.parseInfixExpression)
	p.registerInfix(lexer.STRICT_NOT_EQ, p.parseInfixExpression)
	p.registerInfix(lexer.LT, p.parseInfixExpression)
	p.registerInfix(lexer.GT, p.parseInfixExpression)
	p.registerInfix(lexer.LE, p.parseInfixExpression)
	p.registerInfix(lexer.GE, p.parseInfixExpression)
	p.registerInfix(lexer.IN, p.parseInfixExpression)
	p.registerInfix(lexer.INSTANCEOF, p.parseInfixExpression)
	p.registerInfix(lexer.LOGICAL_AND, p.parseInfixExpression)
	p.registerInfix(lexer.LOGICAL_OR, p.parseInfixExpression)
	p.registerInfix(lexer.COALESCE, p.parseInfixExpression)
	// Bitwise and Shift
	p.registerInfix(lexer.BITWISE_AND, p.parseInfixExpression)
	p.registerInfix(lexer.PIPE, p.parseInfixExpression) // VALUE context: Treat '|' as BITWISE_OR
	p.registerInfix(lexer.BITWISE_XOR, p.parseInfixExpression)
	p.registerInfix(lexer.LEFT_SHIFT, p.parseInfixExpression)
	p.registerInfix(lexer.RIGHT_SHIFT, p.parseInfixExpression)
	p.registerInfix(lexer.UNSIGNED_RIGHT_SHIFT, p.parseInfixExpression)
	// Type Assertion
	p.registerInfix(lexer.AS, p.parseTypeAssertionExpression)

	// Call, Index, Member, Ternary
	p.registerInfix(lexer.LPAREN, p.parseCallExpression)    // Value context: function call
	p.registerInfix(lexer.LBRACKET, p.parseIndexExpression) // Value context: array/member index
	p.registerInfix(lexer.DOT, p.parseMemberExpression)
	p.registerInfix(lexer.OPTIONAL_CHAINING, p.parseOptionalChainingExpression)
	p.registerInfix(lexer.QUESTION, p.parseTernaryExpression)
	// Assignment Operators
	p.registerInfix(lexer.ASSIGN, p.parseAssignmentExpression)
	p.registerInfix(lexer.PLUS_ASSIGN, p.parseAssignmentExpression)
	p.registerInfix(lexer.MINUS_ASSIGN, p.parseAssignmentExpression)
	p.registerInfix(lexer.ASTERISK_ASSIGN, p.parseAssignmentExpression)
	p.registerInfix(lexer.SLASH_ASSIGN, p.parseAssignmentExpression)
	p.registerInfix(lexer.REMAINDER_ASSIGN, p.parseAssignmentExpression)
	p.registerInfix(lexer.EXPONENT_ASSIGN, p.parseAssignmentExpression)
	p.registerInfix(lexer.BITWISE_AND_ASSIGN, p.parseAssignmentExpression)          // &= (New)
	p.registerInfix(lexer.BITWISE_OR_ASSIGN, p.parseAssignmentExpression)           // |= (New)
	p.registerInfix(lexer.BITWISE_XOR_ASSIGN, p.parseAssignmentExpression)          // ^= (New)
	p.registerInfix(lexer.LEFT_SHIFT_ASSIGN, p.parseAssignmentExpression)           // <<= (New)
	p.registerInfix(lexer.RIGHT_SHIFT_ASSIGN, p.parseAssignmentExpression)          // >>= (New)
	p.registerInfix(lexer.UNSIGNED_RIGHT_SHIFT_ASSIGN, p.parseAssignmentExpression) // >>>= (New)
	p.registerInfix(lexer.LOGICAL_AND_ASSIGN, p.parseAssignmentExpression)          // &&= (New)
	p.registerInfix(lexer.LOGICAL_OR_ASSIGN, p.parseAssignmentExpression)           // ||= (New)
	p.registerInfix(lexer.COALESCE_ASSIGN, p.parseAssignmentExpression)             // ??= (New)

	// Postfix Update Operators
	p.registerInfix(lexer.INC, p.parsePostfixUpdateExpression)
	p.registerInfix(lexer.DEC, p.parsePostfixUpdateExpression)

	// --- Register TYPE Prefix Functions ---
	// --- MODIFIED: Use parseTypeIdentifier for simple type names ---
	p.registerTypePrefix(lexer.IDENT, p.parseTypeIdentifier)       // Basic types like 'number', 'string', custom types
	p.registerTypePrefix(lexer.NULL, p.parseNullLiteral)           // 'null' type
	p.registerTypePrefix(lexer.UNDEFINED, p.parseUndefinedLiteral) // 'undefined' type
	p.registerTypePrefix(lexer.VOID, p.parseVoidTypeLiteral)       // 'void' type
	// NEW: Constructor types that start with 'new'
	p.registerTypePrefix(lexer.NEW, p.parseConstructorTypeExpression) // NEW: Constructor types like 'new () => T'
	// Literal types in TYPE context too
	p.registerTypePrefix(lexer.STRING, p.parseStringLiteral)
	p.registerTypePrefix(lexer.NUMBER, p.parseNumberLiteral)
	p.registerTypePrefix(lexer.TRUE, p.parseBooleanLiteral)
	p.registerTypePrefix(lexer.FALSE, p.parseBooleanLiteral)
	// Function types that start with '('
	p.registerTypePrefix(lexer.LPAREN, p.parseFunctionTypeExpression) // Starts with '(', e.g., '() => number'
	// Object type literals that start with '{'
	p.registerTypePrefix(lexer.LBRACE, p.parseObjectTypeExpression) // NEW: Object type literals like { name: string; age: number }
	// --- NEW: Tuple type literals that start with '[' ---
	p.registerTypePrefix(lexer.LBRACKET, p.parseTupleTypeExpression) // NEW: Tuple type literals like [string, number, boolean?]

	// --- Register TYPE Infix Functions ---
	p.registerTypeInfix(lexer.PIPE, p.parseUnionTypeExpression)               // TYPE context: '|' is union
	p.registerTypeInfix(lexer.BITWISE_AND, p.parseIntersectionTypeExpression) // TYPE context: '&' is intersection
	p.registerTypeInfix(lexer.LBRACKET, p.parseArrayTypeExpression)           // TYPE context: 'T[]'

	// Read two tokens, so curToken and peekToken are both set
	p.nextToken()
	p.nextToken()

	return p
}

// Errors returns the list of parsing errors.
func (p *Parser) Errors() []errors.PaseratiError {
	return p.errors
}

// nextToken advances the current and peek tokens.
func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
	debugPrint("nextToken(): cur='%s' (%s), peek='%s' (%s)", p.curToken.Literal, p.curToken.Type, p.peekToken.Literal, p.peekToken.Type)
}

// ParseProgram parses the entire input and returns the root Program node and any errors.
func (p *Parser) ParseProgram() (*Program, []errors.PaseratiError) {
	program := &Program{}
	program.Statements = []Statement{}
	program.HoistedDeclarations = make(map[string]Expression) // Initialize map with Expression

	for p.curToken.Type != lexer.EOF {
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)

			// --- Hoisting Check ---
			// Check if the statement IS an ExpressionStatement containing a FunctionLiteral
			if exprStmt, isExprStmt := stmt.(*ExpressionStatement); isExprStmt {
				if exprStmt.Expression != nil {
					if funcLit, isFuncLit := exprStmt.Expression.(*FunctionLiteral); isFuncLit && funcLit.Name != nil {
						if _, exists := program.HoistedDeclarations[funcLit.Name.Value]; exists {
							// Function with this name already hoisted
							p.addError(funcLit.Name.Token, fmt.Sprintf("duplicate hoisted function declaration: %s", funcLit.Name.Value))
						} else {
							program.HoistedDeclarations[funcLit.Name.Value] = funcLit // Store Expression
						}
					}
				}
			}
			// --- End Hoisting Check ---
		}
		if p.curToken.Type != lexer.EOF {
			p.nextToken()
		} else {
			break
		}
	}

	return program, p.errors
}

// --- Statement Parsing ---

func (p *Parser) parseStatement() Statement {
	switch p.curToken.Type {
	case lexer.LET:
		return p.parseLetStatement()
	case lexer.CONST:
		return p.parseConstStatement()
	case lexer.VAR: // Added case
		return p.parseVarStatement()
	case lexer.RETURN:
		return p.parseReturnStatement()
	case lexer.IF:
		return p.parseIfStatement()
	case lexer.WHILE:
		return p.parseWhileStatement()
	case lexer.DO:
		return p.parseDoWhileStatement()
	case lexer.FOR:
		return p.parseForStatement()
	case lexer.BREAK:
		return p.parseBreakStatement()
	case lexer.CONTINUE:
		return p.parseContinueStatement()
	case lexer.TYPE:
		return p.parseTypeAliasStatement()
	case lexer.INTERFACE:
		return p.parseInterfaceDeclaration()
	case lexer.SWITCH:
		return p.parseSwitchStatement()
	case lexer.FUNCTION:
		return p.parseFunctionDeclarationStatement()
	default:
		return p.parseExpressionStatement()
	}
}

// --- Function Declaration Statement Parsing ---
func (p *Parser) parseFunctionDeclarationStatement() *ExpressionStatement {
	// Parse the function as an expression (FunctionLiteral)
	funcExpr := p.parseFunctionLiteral()
	if funcExpr == nil {
		// If function parsing failed, return an empty expression statement
		// to avoid nil statement that would cause panic in hoisting logic
		return &ExpressionStatement{
			Token:      p.curToken,
			Expression: nil,
		}
	}

	// Wrap it in an ExpressionStatement
	stmt := &ExpressionStatement{
		Token:      p.curToken,
		Expression: funcExpr,
	}

	// Optional semicolon
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

// --- NEW: Type Alias Statement Parsing ---
func (p *Parser) parseTypeAliasStatement() *TypeAliasStatement {
	stmt := &TypeAliasStatement{Token: p.curToken} // 'type' token

	if !p.expectPeek(lexer.IDENT) {
		return nil // Expected identifier after 'type'
	}

	stmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(lexer.ASSIGN) {
		return nil // Expected '=' after identifier
	}

	p.nextToken() // Consume '=', move to the start of the type expression

	stmt.Type = p.parseTypeExpression()
	if stmt.Type == nil {
		return nil // Error parsing the type expression
	}

	// Optional semicolon
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

// --- NEW: Type Expression Parsing ---

// parseTypeExpression parses a type annotation, potentially including union types.
func (p *Parser) parseTypeExpression() Expression {
	// Start parsing with the lowest type precedence
	return p.parseTypeExpressionRecursive(TYPE_LOWEST)
}

// parseTypeExpressionRecursive handles precedence for type operators.
// Uses typePrefixParseFns and typeInfixParseFns.
func (p *Parser) parseTypeExpressionRecursive(precedence int) Expression {
	debugPrint("parseTypeExpressionRecursive(prec=%d): START, cur='%s'", precedence, p.curToken.Literal)

	// --- MODIFIED: Use typePrefixParseFns ---
	prefix := p.typePrefixParseFns[p.curToken.Type]
	if prefix == nil {
		// Error: No function found to start parsing this token as a type
		msg := fmt.Sprintf("unexpected token %s (%q) at start of type annotation",
			p.curToken.Type, p.curToken.Literal)
		p.addError(p.curToken, msg)
		debugPrint("parseTypeExpressionRecursive: ERROR - %s", msg)
		return nil
	}
	leftExp := prefix()

	if leftExp == nil {
		debugPrint("parseTypeExpressionRecursive: type prefix parse returned nil for token %s", p.curToken.Literal)
		return nil // Prefix parsing failed
	}

	debugPrint("parseTypeExpressionRecursive: Parsed prefix type %T ('%s')", leftExp, leftExp.String())

	// --- MODIFIED: Loop using peekTypePrecedence and typeInfixParseFns ---
	for precedence < p.peekTypePrecedence() {
		peekType := p.peekToken.Type
		infix := p.typeInfixParseFns[peekType] // Look in the TYPE infix map
		if infix == nil {
			// No infix type operator found or lower precedence for the peek token
			debugPrint("parseTypeExpressionRecursive: No TYPE infix for peek='%s', returning leftExp=%T", p.peekToken.Literal, leftExp)
			return leftExp
		}

		debugPrint("parseTypeExpressionRecursive: Found TYPE infix for peek='%s' (%s), type precedence=%d", p.peekToken.Literal, peekType, p.peekTypePrecedence())
		p.nextToken() // Consume the type operator token (e.g., '|' or '[')
		debugPrint("parseTypeExpressionRecursive: After infix nextToken(), cur='%s' (%s)", p.curToken.Literal, p.curToken.Type)

		leftExp = infix(leftExp) // Call the specific type infix function (e.g., parseUnionTypeExpression)

		if leftExp == nil {
			debugPrint("parseTypeExpressionRecursive: TYPE infix function returned nil")
			return nil // Infix parsing failed
		}
		debugPrint("parseTypeExpressionRecursive: After TYPE infix call, leftExp=%T, cur='%s', peek='%s'", leftExp, p.curToken.Literal, p.peekToken.Literal)
	}

	debugPrint("parseTypeExpressionRecursive(prec=%d): loop end, returning leftExp=%T", precedence, leftExp)
	return leftExp
}

// --- NEW: Helper for parsing function types like () => T or (A, B) => T ---
// parseFunctionTypeExpression should already call parseTypeExpression, which now uses the recursive helper correctly.
func (p *Parser) parseFunctionTypeExpression() Expression {
	// ... existing implementation looks okay, relies on parseTypeExpression calls ...
	funcType := &FunctionTypeExpression{Token: p.curToken} // '(' token

	var parseErr error
	funcType.Parameters, funcType.RestParameter, parseErr = p.parseFunctionTypeParameterList()
	if parseErr != nil {
		// Error already added by helper
		return nil
	}

	// Expect '=>' after parameter list
	if !p.expectPeek(lexer.ARROW) {
		return nil // Expected ' => '
	}

	p.nextToken()                                 // Consume ' => ', move to the return type
	funcType.ReturnType = p.parseTypeExpression() // This call will use the updated recursive function
	if funcType.ReturnType == nil {
		return nil // Error parsing return type
	}

	return funcType
}

// --- NEW: Helper for parsing function type parameter list: (), (T1), (name: T1, T2) ---
// This function should also correctly use parseTypeExpression internally.
func (p *Parser) parseFunctionTypeParameterList() ([]Expression, Expression, error) {
	// ... existing implementation looks okay, relies on parseTypeExpression calls ...
	params := []Expression{}
	var restParam Expression

	if !p.curTokenIs(lexer.LPAREN) {
		// Should not happen if called correctly
		msg := fmt.Sprintf("internal parser error: parseFunctionTypeParameterList called without LPAREN, got %s", p.curToken.Type)
		p.addError(p.curToken, msg)
		return nil, nil, fmt.Errorf("%s", msg)
	}

	// Handle empty parameter list: () => ...
	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken() // Consume ')'
		return params, nil, nil
	}

	// Parse first parameter type
	p.nextToken() // Consume '('

	// Check for rest parameter
	if p.curTokenIs(lexer.SPREAD) {
		// This is a rest parameter: ...type
		restParam = p.parseRestParameterType()
		if restParam == nil {
			return nil, nil, fmt.Errorf("failed to parse rest parameter type")
		}
		// Expect closing parenthesis after rest parameter
		if !p.expectPeek(lexer.RPAREN) {
			return nil, nil, fmt.Errorf("missing closing parenthesis after rest parameter")
		}
		return params, restParam, nil
	}

	// --- MODIFIED: Handle optional parameter name ---
	if p.curTokenIs(lexer.IDENT) && p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume IDENT (parameter name, ignored for type)
		p.nextToken() // Consume ':', move to the actual type
	} // Now curToken should be the start of the type expression
	// --- END MODIFICATION ---

	paramType := p.parseTypeExpression() // This call will use the updated recursive function
	if paramType == nil {
		return nil, nil, fmt.Errorf("failed to parse first function type parameter")
	}
	params = append(params, paramType)

	// Parse subsequent parameter types
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken() // Consume ','
		p.nextToken() // Move to next token (could be IDENT or start of type)

		// Check for rest parameter
		if p.curTokenIs(lexer.SPREAD) {
			// This is a rest parameter: ...type
			restParam = p.parseRestParameterType()
			if restParam == nil {
				return nil, nil, fmt.Errorf("failed to parse rest parameter type")
			}
			// Expect closing parenthesis after rest parameter
			if !p.expectPeek(lexer.RPAREN) {
				return nil, nil, fmt.Errorf("missing closing parenthesis after rest parameter")
			}
			return params, restParam, nil
		}

		// --- MODIFIED: Handle optional parameter name ---
		if p.curTokenIs(lexer.IDENT) && p.peekTokenIs(lexer.COLON) {
			p.nextToken() // Consume IDENT
			p.nextToken() // Consume ':', move to the actual type
		} // Now curToken should be the start of the type expression
		// --- END MODIFICATION ---

		paramType := p.parseTypeExpression() // This call will use the updated recursive function
		if paramType == nil {
			return nil, nil, fmt.Errorf("failed to parse subsequent function type parameter")
		}
		params = append(params, paramType)
	}

	// Expect closing parenthesis
	if !p.expectPeek(lexer.RPAREN) {
		return nil, nil, fmt.Errorf("missing closing parenthesis in function type parameter list")
	}

	return params, restParam, nil
}

// parseRestParameterType parses a rest parameter type like ...args: string[]
// In function type expressions, the parameter name is optional and can be ignored
func (p *Parser) parseRestParameterType() Expression {
	if !p.curTokenIs(lexer.SPREAD) {
		p.addError(p.curToken, "expected '...' for rest parameter")
		return nil
	}

	// Move past the '...' token
	p.nextToken()

	// Check if there's a parameter name (optional in type expressions)
	if p.curTokenIs(lexer.IDENT) {
		// Skip the parameter name - we don't need it in type expressions
		p.nextToken()
	}

	// Check for type annotation
	if p.curTokenIs(lexer.COLON) {
		p.nextToken() // Consume ':'
		// Parse the type (should be an array type)
		restType := p.parseTypeExpression()
		if restType == nil {
			p.addError(p.curToken, "expected type annotation after ':' in rest parameter type")
			return nil
		}
		return restType
	} else {
		// No type annotation - default to any[]
		// Return an ArrayTypeExpression with 'any' as element type
		anyType := &Identifier{
			Token: lexer.Token{Type: lexer.IDENT, Literal: "any"},
			Value: "any",
		}
		return &ArrayTypeExpression{
			Token:       p.curToken,
			ElementType: anyType,
		}
	}
}

// --- NEW: Helper for infix union type parsing ---
// This function should also correctly use parseTypeExpressionRecursive internally.
func (p *Parser) parseUnionTypeExpression(left Expression) Expression {
	// ... existing implementation looks okay ...
	unionExp := &UnionTypeExpression{
		Token: p.curToken, // The '|' token
		Left:  left,
	}
	// Use the precedence of the UNION operator itself for the recursive call
	precedence := TYPE_UNION
	p.nextToken()                                               // Consume the token starting the right-hand side type
	unionExp.Right = p.parseTypeExpressionRecursive(precedence) // Recursive call uses type precedence
	if unionExp.Right == nil {
		return nil // Error parsing right side
	}
	return unionExp
}

// --- NEW: Helper for infix intersection type parsing ---
// This function handles intersection types like A & B
func (p *Parser) parseIntersectionTypeExpression(left Expression) Expression {
	intersectionExp := &IntersectionTypeExpression{
		Token: p.curToken, // The '&' token
		Left:  left,
	}
	// Use the precedence of the INTERSECTION operator itself for the recursive call
	precedence := TYPE_INTERSECTION
	p.nextToken()                                                      // Consume the token starting the right-hand side type
	intersectionExp.Right = p.parseTypeExpressionRecursive(precedence) // Recursive call uses type precedence
	if intersectionExp.Right == nil {
		return nil // Error parsing right side
	}
	return intersectionExp
}

// --- NEW: Precedence helper for type operators ---
func (p *Parser) peekTypePrecedence() int {
	// Look in the type precedences map
	if prec, ok := typePrecedences[p.peekToken.Type]; ok {
		return prec
	}
	return TYPE_LOWEST
}

// --- NEW: Helper for infix array type parsing T[] ---
// This function does not need recursion.
func (p *Parser) parseArrayTypeExpression(elementType Expression) Expression {
	// ... existing implementation looks okay ...
	arrayTypeExp := &ArrayTypeExpression{
		Token:       p.curToken, // The '[' token
		ElementType: elementType,
	}
	// We expect immediate RBRACKET for T[] syntax
	if !p.expectPeek(lexer.RBRACKET) {
		return nil // Expected ']' after '[' for array type
	}
	return arrayTypeExp
}

// --- NEW: Helper for parsing tuple types [T, U, V] ---
func (p *Parser) parseTupleTypeExpression() Expression {
	tupleTypeExp := &TupleTypeExpression{
		Token:         p.curToken, // The '[' token
		ElementTypes:  []Expression{},
		OptionalFlags: []bool{},
		RestElement:   nil,
	}

	debugPrint("parseTupleTypeExpression: Starting, cur='%s'", p.curToken.Literal)

	// Check if this is an empty tuple []
	if p.peekTokenIs(lexer.RBRACKET) {
		p.nextToken() // Move to ']'
		debugPrint("parseTupleTypeExpression: Empty tuple")
		return tupleTypeExp
	}

	// Parse element list - advance to the first element
	p.nextToken() // Move past '['

	for !p.curTokenIs(lexer.RBRACKET) {
		debugPrint("parseTupleTypeExpression: Parsing element, cur='%s'", p.curToken.Literal)

		// Check for rest element syntax (...T[])
		if p.curTokenIs(lexer.SPREAD) {
			// Parse rest element: '...T[]'
			p.nextToken() // Move past '...' to the type

			restType := p.parseTypeExpression()
			if restType == nil {
				p.addError(p.curToken, "expected type after '...' in tuple rest element")
				return nil
			}
			tupleTypeExp.RestElement = restType
			debugPrint("parseTupleTypeExpression: Parsed rest element: %s", restType.String())

			// After rest element, we must have either ',' followed by ']' or just ']'
			if p.peekTokenIs(lexer.COMMA) {
				p.nextToken() // Consume ','
				if !p.peekTokenIs(lexer.RBRACKET) {
					p.addError(p.peekToken, "rest element must be the last element in tuple type")
					return nil
				}
				p.nextToken() // Move to ']'
			} else if !p.peekTokenIs(lexer.RBRACKET) {
				p.addError(p.peekToken, "expected ',' or ']' after rest element in tuple type")
				return nil
			} else {
				p.nextToken() // Move to ']'
			}
			break
		}

		// Parse regular element type
		elemType := p.parseTypeExpression()
		if elemType == nil {
			return nil
		}

		tupleTypeExp.ElementTypes = append(tupleTypeExp.ElementTypes, elemType)

		// Check for optional marker '?'
		isOptional := false
		if p.peekTokenIs(lexer.QUESTION) {
			isOptional = true
			p.nextToken() // Consume '?'
		}
		tupleTypeExp.OptionalFlags = append(tupleTypeExp.OptionalFlags, isOptional)

		debugPrint("parseTupleTypeExpression: Parsed element %d: %s (optional: %v)",
			len(tupleTypeExp.ElementTypes)-1, elemType.String(), isOptional)

		// Check for comma or closing bracket
		if p.peekTokenIs(lexer.COMMA) {
			p.nextToken() // Consume ','
			p.nextToken() // Move to next element
		} else if p.peekTokenIs(lexer.RBRACKET) {
			p.nextToken() // Move to ']'
			break
		} else {
			p.addError(p.peekToken, "expected ',' or ']' in tuple type")
			return nil
		}
	}

	// We should now be at ']'
	if !p.curTokenIs(lexer.RBRACKET) {
		p.addError(p.curToken, "expected ']' to close tuple type")
		return nil
	}

	debugPrint("parseTupleTypeExpression: Completed, elements: %d, rest: %v",
		len(tupleTypeExp.ElementTypes), tupleTypeExp.RestElement != nil)

	return tupleTypeExp
}

func (p *Parser) parseLetStatement() *LetStatement {
	stmt := &LetStatement{Token: p.curToken}

	if !p.expectPeek(lexer.IDENT) {
		return nil
	}

	stmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Optional Type Annotation
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ':'
		p.nextToken() // Consume token starting the type expression
		// --- MODIFIED: Use parseTypeExpression ---
		stmt.TypeAnnotation = p.parseTypeExpression()
		if stmt.TypeAnnotation == nil {
			return nil
		} // Propagate parsing error
	} else {
		stmt.TypeAnnotation = nil // No type annotation provided
	}

	// Allow omitting = value, defaulting to undefined
	if p.peekTokenIs(lexer.ASSIGN) {
		p.nextToken() // Consume '='
		p.nextToken() // Consume token starting the expression
		stmt.Value = p.parseExpression(LOWEST)
	} else {
		stmt.Value = nil // No initializer provided, implies undefined
	}

	// Optional semicolon - Consume it here
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseConstStatement() *ConstStatement {
	stmt := &ConstStatement{Token: p.curToken}

	if !p.expectPeek(lexer.IDENT) {
		return nil
	}

	stmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Optional Type Annotation
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ':'
		p.nextToken() // Consume the token starting the type expression
		// --- MODIFIED: Use parseTypeExpression ---
		stmt.TypeAnnotation = p.parseTypeExpression()
		if stmt.TypeAnnotation == nil {
			return nil
		} // Propagate parsing error
	} else {
		stmt.TypeAnnotation = nil // No type annotation provided
	}

	if !p.expectPeek(lexer.ASSIGN) {
		return nil
	}

	p.nextToken() // Consume '='

	stmt.Value = p.parseExpression(LOWEST)

	// Optional semicolon - Consume it here
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseVarStatement() *VarStatement {
	stmt := &VarStatement{Token: p.curToken}

	if !p.expectPeek(lexer.IDENT) {
		return nil
	}

	stmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Optional Type Annotation
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ':'
		p.nextToken() // Consume token starting the type expression
		stmt.TypeAnnotation = p.parseTypeExpression()
		if stmt.TypeAnnotation == nil {
			return nil
		}
	} else {
		stmt.TypeAnnotation = nil // No type annotation provided
	}

	// Allow omitting = value, defaulting to undefined
	if p.peekTokenIs(lexer.ASSIGN) {
		p.nextToken() // Consume '='
		p.nextToken() // Consume token starting the expression
		stmt.Value = p.parseExpression(LOWEST)
	} else {
		stmt.Value = nil // No initializer provided, implies undefined
	}

	// Optional semicolon - Consume it here
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseReturnStatement() *ReturnStatement {
	stmt := &ReturnStatement{Token: p.curToken}
	p.nextToken() // Consume 'return'

	if p.curTokenIs(lexer.SEMICOLON) {
		// Handle 'return;' explicitly by setting nil and consuming ';'
		stmt.ReturnValue = nil
		// curToken is already ';', main loop will advance
	} else if p.curTokenIs(lexer.RBRACE) || p.curTokenIs(lexer.EOF) {
		// Handle 'return}' or 'return<EOF>' - no expression, no semicolon to consume
		stmt.ReturnValue = nil
	} else {
		// Parse the expression
		stmt.ReturnValue = p.parseExpression(LOWEST)
		// Optional semicolon - Consume it here
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken()
		}
	}

	return stmt
}

func (p *Parser) parseIfStatement() *IfStatement {
	stmt := &IfStatement{Token: p.curToken} // 'if' token

	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	p.nextToken() // Consume '(', move to condition
	stmt.Condition = p.parseExpression(LOWEST)
	if stmt.Condition == nil {
		return nil
	}

	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}

	// --- MODIFIED: Handle both block statements and single statements ---
	if p.peekTokenIs(lexer.LBRACE) {
		// Block statement case: if (condition) { ... }
		if !p.expectPeek(lexer.LBRACE) {
			return nil
		}
		stmt.Consequence = p.parseBlockStatement()
	} else {
		// Single statement case: if (condition) statement
		p.nextToken() // Move to the start of the statement
		consequenceStmt := p.parseStatement()
		if consequenceStmt == nil {
			return nil
		}
		// Wrap the single statement in a BlockStatement
		stmt.Consequence = &BlockStatement{
			Token:               p.curToken,
			Statements:          []Statement{consequenceStmt},
			HoistedDeclarations: make(map[string]Expression),
		}
	}
	// --- END MODIFICATION ---

	if stmt.Consequence == nil {
		return nil
	}

	// Check for 'else' clause
	if p.peekTokenIs(lexer.ELSE) {
		p.nextToken() // Consume 'else'

		if p.peekTokenIs(lexer.IF) {
			// Handle 'else if' by recursively parsing another if statement
			p.nextToken() // Move to 'if'
			elseIfStmt := p.parseIfStatement()
			if elseIfStmt == nil {
				return nil
			}
			// Wrap the else-if in a block statement for consistency
			stmt.Alternative = &BlockStatement{
				Token:               elseIfStmt.Token,
				Statements:          []Statement{elseIfStmt},
				HoistedDeclarations: make(map[string]Expression),
			}
		} else if p.peekTokenIs(lexer.LBRACE) {
			// Standard 'else' block
			p.nextToken() // Move to '{'
			stmt.Alternative = p.parseBlockStatement()
			if stmt.Alternative == nil {
				return nil
			}
		} else {
			// --- NEW: Single statement case: else statement ---
			p.nextToken() // Move to the start of the else statement
			elseStmt := p.parseStatement()
			if elseStmt == nil {
				return nil
			}
			// Wrap the single statement in a BlockStatement
			stmt.Alternative = &BlockStatement{
				Token:               p.curToken,
				Statements:          []Statement{elseStmt},
				HoistedDeclarations: make(map[string]Expression),
			}
			// --- END NEW ---
		}
	}

	return stmt
}

func (p *Parser) parseExpressionStatement() *ExpressionStatement {
	stmt := &ExpressionStatement{Token: p.curToken}

	stmt.Expression = p.parseExpression(LOWEST)

	// Optional semicolon - Consume it here
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

// --- Expression Parsing (Pratt Parser) ---

func (p *Parser) parseExpression(precedence int) Expression {
	debugPrint("parseExpression(prec=%d): cur='%s' (%s)", precedence, p.curToken.Literal, p.curToken.Type)
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}
	leftExp := prefix()
	// --- NIL CHECK AFTER PREFIX ---
	if leftExp == nil {
		debugPrint("parseExpression(prec=%d): prefix function for '%s' returned nil", precedence, p.curToken.Literal)
		return nil // Prefix parsing failed, propagate nil
	}
	debugPrint("parseExpression(prec=%d): after prefix, leftExp=%T, cur='%s', peek='%s'", precedence, leftExp, p.curToken.Literal, p.peekToken.Literal)

	for !p.peekTokenIs(lexer.SEMICOLON) && precedence < p.peekPrecedence() {
		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			debugPrint("parseExpression(prec=%d): no infix for peek='%s', returning leftExp=%T", precedence, leftExp, p.peekToken.Literal, leftExp)
			return leftExp
		}

		debugPrint("parseExpression(prec=%d): found infix for peek='%s' (%s), precedence=%d", precedence, p.peekToken.Literal, p.peekToken.Type, p.peekPrecedence())
		p.nextToken()
		debugPrint("parseExpression(prec=%d): after infix nextToken(), cur='%s' (%s)", precedence, p.curToken.Literal, p.curToken.Type)

		leftExp = infix(leftExp)
		// --- NIL CHECK AFTER INFIX ---
		if leftExp == nil {
			// This shouldn't happen if infix functions correctly handle their errors,
			// but check defensively.
			debugPrint("parseExpression(prec=%d): infix function returned nil", precedence)
			return nil
		}
		debugPrint("parseExpression(prec=%d): after infix call, leftExp=%T, cur='%s', peek='%s'", precedence, leftExp, p.curToken.Literal, p.peekToken.Literal)
	}

	debugPrint("parseExpression(prec=%d): loop end, returning leftExp=%T", precedence, leftExp)
	return leftExp
}

// -- Prefix Parse Functions --

func (p *Parser) parseIdentifier() Expression {
	ident := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	debugPrint("parseIdentifier (VALUE context): cur='%s', peek='%s' (%s)", p.curToken.Literal, p.peekToken.Literal, p.peekToken.Type)

	// Check ONLY for shorthand arrow function `ident => body` in VALUE context
	if p.peekTokenIs(lexer.ARROW) {
		debugPrint("parseIdentifier (VALUE context): Found '=>' after identifier '%s'", ident.Value)
		p.nextToken() // Consume the identifier token (which is curToken)
		debugPrint("parseIdentifier (VALUE context): Consumed IDENT, cur is now '%s' (%s)", p.curToken.Literal, p.curToken.Type)
		param := &Parameter{
			Token:          ident.Token,
			Name:           ident,
			TypeAnnotation: nil, // No type annotation in this shorthand syntax
		}
		// parseArrowFunctionBodyAndFinish expects curToken to be '=>'
		return p.parseArrowFunctionBodyAndFinish([]*Parameter{param}, nil, nil)
	}

	debugPrint("parseIdentifier (VALUE context): Just identifier '%s', returning.", ident.Value)
	return ident
}

func (p *Parser) parseNumberLiteral() Expression {
	lit := &NumberLiteral{Token: p.curToken}

	rawLiteral := p.curToken.Literal
	base := 10
	prefixLen := 0

	isFloat := false

	// Determine base and prefix length
	if strings.HasPrefix(rawLiteral, "0x") || strings.HasPrefix(rawLiteral, "0X") {
		base = 16
		prefixLen = 2
	} else if strings.HasPrefix(rawLiteral, "0b") || strings.HasPrefix(rawLiteral, "0B") {
		base = 2
		prefixLen = 2
	} else if strings.HasPrefix(rawLiteral, "0o") || strings.HasPrefix(rawLiteral, "0O") {
		base = 8
		prefixLen = 2
	} else if len(rawLiteral) > 1 && rawLiteral[0] == '0' && rawLiteral[1] >= '0' && rawLiteral[1] <= '7' {
		// Handle legacy octal (e.g., 0777) - Check if still desired
		// base = 8
		// prefixLen = 1 // Or 0 if we treat it just as decimal
		// For now, treat as decimal if no 0o prefix.
	}

	// Clean the literal: remove prefix and separators
	numberPart := rawLiteral[prefixLen:]
	cleanedLiteral := strings.ReplaceAll(numberPart, "_", "")

	// Check if it looks like a float (contains ., e, or E) - only relevant for base 10
	if base == 10 && (strings.Contains(cleanedLiteral, ".") || strings.ContainsAny(cleanedLiteral, "eE")) {
		isFloat = true
	}

	// Attempt to parse
	if isFloat {
		value, err := strconv.ParseFloat(cleanedLiteral, 64)
		if err != nil {
			// This suggests the lexer allowed an invalid float format (e.g., "1.2.3", "1e-e")
			msg := fmt.Sprintf("could not parse %q as float64: %v", rawLiteral, err)
			p.addError(p.curToken, msg)
			return nil
		}
		lit.Value = value
	} else {
		// Parse as integer first
		value, err := strconv.ParseInt(cleanedLiteral, base, 64)
		if err != nil {
			// This suggests the lexer allowed invalid digits for the base or invalid format
			msg := fmt.Sprintf("could not parse %q as int (base %d): %v", rawLiteral, base, err)
			p.addError(p.curToken, msg)
			return nil
		}
		// Store as float64 in the AST for simplicity/consistency
		lit.Value = float64(value)
	}

	return lit
}

func (p *Parser) parseStringLiteral() Expression {
	return &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
}

// parseTemplateLiteral parses template literals with interpolations
// Expects current token to be TEMPLATE_START (`), processes tokens in sequence,
// ends with TEMPLATE_END (`). Always maintains string/expression alternation.
func (p *Parser) parseTemplateLiteral() Expression {
	lit := &TemplateLiteral{Token: p.curToken} // TEMPLATE_START token
	lit.Parts = []Node{}

	// Consume the opening backtick
	p.nextToken()

	// Always start with a string part (can be empty)
	expectingString := true

	for !p.curTokenIs(lexer.TEMPLATE_END) && !p.curTokenIs(lexer.EOF) {
		if p.curTokenIs(lexer.TEMPLATE_STRING) {
			if !expectingString {
				p.addError(p.curToken, "unexpected string in template literal")
				return nil
			}
			// String part of the template
			stringPart := &TemplateStringPart{Value: p.curToken.Literal}
			lit.Parts = append(lit.Parts, stringPart)
			expectingString = false
			p.nextToken()
		} else if p.curTokenIs(lexer.TEMPLATE_INTERPOLATION) {
			// If we were expecting a string but got interpolation, add empty string
			if expectingString {
				emptyString := &TemplateStringPart{Value: ""}
				lit.Parts = append(lit.Parts, emptyString)
			}

			p.nextToken() // Move past ${

			// Parse the expression inside the interpolation
			expr := p.parseExpression(LOWEST)
			if expr == nil {
				p.addError(p.curToken, "failed to parse expression in template interpolation")
				return nil
			}
			lit.Parts = append(lit.Parts, expr)

			// Expect closing brace }
			if !p.expectPeek(lexer.RBRACE) {
				p.addError(p.curToken, "expected '}' to close template interpolation")
				return nil
			}
			p.nextToken()          // Move past }
			expectingString = true // After expression, we expect a string
		} else {
			// Unexpected token
			p.addError(p.curToken, fmt.Sprintf("unexpected token in template literal: %s", p.curToken.Type))
			return nil
		}
	}

	if !p.curTokenIs(lexer.TEMPLATE_END) {
		p.addError(p.curToken, "unterminated template literal, expected closing backtick")
		return nil
	}

	// If we were expecting a string at the end, add empty string
	if expectingString {
		emptyString := &TemplateStringPart{Value: ""}
		lit.Parts = append(lit.Parts, emptyString)
	}

	// Don't consume the closing backtick here - let the caller handle it
	return lit
}

func (p *Parser) parseBooleanLiteral() Expression {
	return &BooleanLiteral{Token: p.curToken, Value: p.curTokenIs(lexer.TRUE)}
}

func (p *Parser) parseNullLiteral() Expression {
	return &NullLiteral{Token: p.curToken}
}

func (p *Parser) parseUndefinedLiteral() Expression {
	return &UndefinedLiteral{Token: p.curToken}
}

func (p *Parser) parseThisExpression() Expression {
	return &ThisExpression{Token: p.curToken}
}

func (p *Parser) parseNewExpression() Expression {
	ne := &NewExpression{Token: p.curToken} // 'new' token

	// Move to the next token (constructor identifier/expression)
	p.nextToken()

	// Parse the constructor expression (identifier, member expression, etc.)
	ne.Constructor = p.parseExpression(CALL)

	// Check if there are arguments (parentheses)
	if p.peekTokenIs(lexer.LPAREN) {
		p.nextToken() // Move to '('
		ne.Arguments = p.parseExpressionList(lexer.RPAREN)
	} else {
		// No arguments provided (e.g., "new Date")
		ne.Arguments = []Expression{}
	}

	return ne
}

func (p *Parser) parseFunctionLiteral() Expression {
	lit := &FunctionLiteral{Token: p.curToken}

	// Optional Function Name
	if p.peekTokenIs(lexer.IDENT) {
		p.nextToken() // Consume name identifier
		// Assuming parseIdentifier correctly returns an *Identifier here
		nameIdentExpr := p.parseIdentifier()
		nameIdent, ok := nameIdentExpr.(*Identifier)
		if !ok {
			msg := fmt.Sprintf("expected identifier for function name, got %s", p.curToken.Type)
			p.addError(p.curToken, msg)
			return nil
		}
		lit.Name = nameIdent
	}

	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	// --- MODIFIED: Use parseFunctionParameters ---
	lit.Parameters, lit.RestParameter, _ = p.parseFunctionParameters() // Includes consuming RPAREN
	if lit.Parameters == nil && lit.RestParameter == nil {
		return nil
	} // Propagate error

	// Optional Return Type Annotation
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ':'
		p.nextToken() // Consume the token starting the type expression
		// --- MODIFIED: Use parseTypeExpression ---
		lit.ReturnTypeAnnotation = p.parseTypeExpression()
		if lit.ReturnTypeAnnotation == nil {
			return nil
		} // Propagate parsing error
	} else {
		lit.ReturnTypeAnnotation = nil // No annotation provided
	}

	// Check if this is a function signature (ends with semicolon) or implementation (has body)
	if p.peekTokenIs(lexer.SEMICOLON) {
		// This is a function signature, not an implementation - return FunctionSignature instead
		p.nextToken() // Consume semicolon

		sig := &FunctionSignature{
			Token:                lit.Token,
			Name:                 lit.Name,
			Parameters:           lit.Parameters,
			RestParameter:        lit.RestParameter,
			ReturnTypeAnnotation: lit.ReturnTypeAnnotation,
		}
		return sig
	}

	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	lit.Body = p.parseBlockStatement() // Includes consuming RBRACE

	return lit
}

// --- NEW: Parse function signature (overload declaration without body) ---
func (p *Parser) parseFunctionSignature() *FunctionSignature {
	sig := &FunctionSignature{Token: p.curToken} // 'function' token

	// Function name is required for overloads
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}

	nameIdentExpr := p.parseIdentifier()
	nameIdent, ok := nameIdentExpr.(*Identifier)
	if !ok {
		msg := fmt.Sprintf("expected identifier for function name, got %s", p.curToken.Type)
		p.addError(p.curToken, msg)
		return nil
	}
	sig.Name = nameIdent

	// Don't expectPeek here - parseFunctionParameters expects to see LPAREN in peek
	if !p.peekTokenIs(lexer.LPAREN) {
		msg := fmt.Sprintf("expected '(' after function name, got %s", p.peekToken.Type)
		p.addError(p.peekToken, msg)
		return nil
	}

	// Parse parameters
	sig.Parameters, sig.RestParameter, _ = p.parseFunctionParameters()
	if sig.Parameters == nil && sig.RestParameter == nil {
		return nil
	}

	// Return type annotation is required for overload signatures
	if !p.peekTokenIs(lexer.COLON) {
		msg := "function overload signatures must have return type annotations"
		p.addError(p.curToken, msg)
		return nil
	}

	p.nextToken() // Consume ':'
	p.nextToken() // Consume the token starting the type expression
	sig.ReturnTypeAnnotation = p.parseTypeExpression()
	if sig.ReturnTypeAnnotation == nil {
		return nil
	}

	// Expect semicolon to end the signature
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken() // Consume semicolon
	}

	return sig
}

// --- MODIFIED: parseFunctionParameters to handle Parameter struct & types ---
// Returns ([]*Parameter, *RestParameter)
func (p *Parser) parseFunctionParameters() ([]*Parameter, *RestParameter, error) {
	parameters := []*Parameter{}
	var restParam *RestParameter

	// Check for empty parameter list: function() { ... }
	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken() // Consume ')'
		return parameters, nil, nil
	}

	p.nextToken() // Consume '(' or ',' to get to the first parameter name

	// Check if first parameter is a rest parameter
	if p.curTokenIs(lexer.SPREAD) {
		// Parse rest parameter
		restParam = p.parseRestParameter()
		if restParam == nil {
			return nil, nil, fmt.Errorf("failed to parse rest parameter")
		}
		if !p.expectPeek(lexer.RPAREN) {
			return nil, nil, fmt.Errorf("expected closing parenthesis after rest parameter")
		}
		return parameters, restParam, nil
	}

	// Parse first regular parameter (could be 'this' parameter)
	if !p.curTokenIs(lexer.IDENT) && !p.curTokenIs(lexer.THIS) {
		msg := fmt.Sprintf("expected identifier or 'this' for parameter name, got %s", p.curToken.Type)
		p.addError(p.curToken, msg)
		debugPrint("parseParameterList: Error - %s", msg)
		return nil, nil, fmt.Errorf("%s", msg)
	}
	param := &Parameter{Token: p.curToken}

	// Check if this is an explicit 'this' parameter
	if p.curTokenIs(lexer.THIS) {
		param.IsThis = true
		param.Name = nil // 'this' parameters don't have a name field

		// 'this' parameters are never optional
		if p.peekTokenIs(lexer.QUESTION) {
			p.addError(p.peekToken, "'this' parameter cannot be optional")
			return nil, nil, fmt.Errorf("'this' parameter cannot be optional")
		}

		// 'this' parameters must have a type annotation
		if !p.peekTokenIs(lexer.COLON) {
			p.addError(p.peekToken, "'this' parameter must have a type annotation")
			return nil, nil, fmt.Errorf("'this' parameter must have a type annotation")
		}
	} else {
		param.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

	// Check for optional parameter (?)
	if p.peekTokenIs(lexer.QUESTION) {
		if param.IsThis {
			// Already handled above
		} else {
			p.nextToken() // Consume '?'
			param.Optional = true
		}
	}

	// Check for Type Annotation
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ':'
		p.nextToken() // Consume token starting the type expression
		param.TypeAnnotation = p.parseTypeExpression()
		if param.TypeAnnotation == nil {
			return nil, nil, fmt.Errorf("failed to parse type annotation for parameter")
		} // Propagate error
	} else {
		if param.IsThis {
			// Already handled above
		} else {
			param.TypeAnnotation = nil
		}
	}

	// Check for Default Value
	if p.peekTokenIs(lexer.ASSIGN) {
		if param.IsThis {
			p.addError(p.peekToken, "'this' parameter cannot have a default value")
			return nil, nil, fmt.Errorf("'this' parameter cannot have a default value")
		} else {
			p.nextToken() // Consume '='
			p.nextToken() // Move to expression
			param.DefaultValue = p.parseExpression(LOWEST)
			if param.DefaultValue == nil {
				p.addError(p.curToken, "expected expression after '=' in parameter default value")
				return nil, nil, fmt.Errorf("expected expression after '=' in parameter default value")
			}
		}
	}

	parameters = append(parameters, param)

	// Parse subsequent parameters (comma-separated)
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken() // Consume ','
		p.nextToken() // Consume identifier for next param name

		// Check if this is a rest parameter
		if p.curTokenIs(lexer.SPREAD) {
			// Parse rest parameter (must be last)
			restParam = p.parseRestParameter()
			if restParam == nil {
				return nil, nil, fmt.Errorf("failed to parse rest parameter")
			}
			// Expect closing parenthesis after rest parameter
			if !p.expectPeek(lexer.RPAREN) {
				return nil, nil, fmt.Errorf("expected closing parenthesis after rest parameter")
			}
			return parameters, restParam, nil
		}

		// 'this' can only be the first parameter
		if p.curTokenIs(lexer.THIS) {
			p.addError(p.curToken, "'this' parameter can only be the first parameter")
			return nil, nil, fmt.Errorf("'this' parameter can only be the first parameter")
		}

		if !p.curTokenIs(lexer.IDENT) {
			msg := fmt.Sprintf("expected identifier for parameter name after comma, got %s", p.curToken.Type)
			p.addError(p.curToken, msg)
			debugPrint("parseParameterList: Error - %s", msg)
			return nil, nil, fmt.Errorf("%s", msg)
		}
		param := &Parameter{Token: p.curToken}
		param.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

		// Check for optional parameter (?)
		if p.peekTokenIs(lexer.QUESTION) {
			p.nextToken() // Consume '?'
			param.Optional = true
		}

		// Check for Type Annotation
		if p.peekTokenIs(lexer.COLON) {
			p.nextToken() // Consume ':'
			p.nextToken() // Consume token starting the type expression
			param.TypeAnnotation = p.parseTypeExpression()
			if param.TypeAnnotation == nil {
				return nil, nil, fmt.Errorf("failed to parse type annotation for parameter")
			} // Propagate error
		} else {
			param.TypeAnnotation = nil
		}

		// Check for Default Value
		if p.peekTokenIs(lexer.ASSIGN) {
			p.nextToken() // Consume '='
			p.nextToken() // Move to expression
			param.DefaultValue = p.parseExpression(LOWEST)
			if param.DefaultValue == nil {
				p.addError(p.curToken, "expected expression after '=' in parameter default value")
				return nil, nil, fmt.Errorf("expected expression after '=' in parameter default value")
			}
		}

		parameters = append(parameters, param)
	}

	if !p.expectPeek(lexer.RPAREN) {
		return nil, nil, fmt.Errorf("expected closing parenthesis after parameters")
	}

	return parameters, restParam, nil
}

// parseRestParameter parses a rest parameter (...args or ...args: type)
func (p *Parser) parseRestParameter() *RestParameter {
	restParam := &RestParameter{Token: p.curToken} // The '...' token

	// Expect identifier after '...'
	if !p.expectPeek(lexer.IDENT) {
		p.addError(p.peekToken, "expected identifier after '...' in rest parameter")
		return nil
	}

	restParam.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check for type annotation
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ':'
		p.nextToken() // Move to type expression
		restParam.TypeAnnotation = p.parseTypeExpression()
		if restParam.TypeAnnotation == nil {
			return nil
		}
	}

	return restParam
}

func (p *Parser) parseSpreadElement() Expression {
	spreadElement := &SpreadElement{Token: p.curToken} // The '...' token

	// Parse the expression after '...'
	p.nextToken() // Move to the expression
	spreadElement.Argument = p.parseExpression(LOWEST)
	if spreadElement.Argument == nil {
		p.addError(p.curToken, "expected expression after '...' in spread syntax")
		return nil
	}

	return spreadElement
}

func (p *Parser) parseBlockStatement() *BlockStatement {
	block := &BlockStatement{Token: p.curToken} // The '{' token
	block.Statements = []Statement{}
	block.HoistedDeclarations = make(map[string]Expression) // Initialize map with Expression

	p.nextToken() // Consume '{'

	for !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)

			// --- Hoisting Check ---
			// Check if the statement IS an ExpressionStatement containing a FunctionLiteral
			if exprStmt, isExprStmt := stmt.(*ExpressionStatement); isExprStmt {
				if funcLit, isFuncLit := exprStmt.Expression.(*FunctionLiteral); isFuncLit && funcLit.Name != nil {
					if _, exists := block.HoistedDeclarations[funcLit.Name.Value]; exists {
						// Function with this name already hoisted in this block
						p.addError(funcLit.Name.Token, fmt.Sprintf("duplicate hoisted function declaration in block: %s", funcLit.Name.Value)) // Use Token
					} else {
						block.HoistedDeclarations[funcLit.Name.Value] = funcLit // Store Expression
					}
				}
			}
			// --- End Hoisting Check ---
		}
		p.nextToken()
	}

	if !p.curTokenIs(lexer.RBRACE) {
		// If we exited the loop because of EOF, RBRACE is missing
		p.peekError(lexer.RBRACE) // Report missing RBRACE error
		return nil
	}

	// Current token is RBRACE, don't consume it here, let the caller (e.g. parseFunctionLiteral) handle it or the main loop advance.

	// --- DEBUG: Log block state before returning ---
	statementsPtr := &block.Statements // Get pointer to the slice header itself
	if debugParser {
		debugPrint("// [Parser Debug] Returning Block: Ptr=%p, Statements Slice Header Ptr=%p", block, statementsPtr)
		if block.Statements == nil {
			fmt.Printf(", Statements=nil\n")
		} else {
			fmt.Printf(", Statements.Len=%d\n", len(block.Statements))
		}
	}
	// --- END DEBUG ---

	return block
}

// --- Helper Methods ---

func (p *Parser) registerPrefix(tokenType lexer.TokenType, fn prefixParseFn) {
	p.prefixParseFns[tokenType] = fn
}

func (p *Parser) registerInfix(tokenType lexer.TokenType, fn infixParseFn) {
	p.infixParseFns[tokenType] = fn
}

// --- NEW: Helper methods for TYPE parsing functions ---
func (p *Parser) registerTypePrefix(tokenType lexer.TokenType, fn prefixParseFn) {
	p.typePrefixParseFns[tokenType] = fn
}

func (p *Parser) registerTypeInfix(tokenType lexer.TokenType, fn infixParseFn) {
	p.typeInfixParseFns[tokenType] = fn
}

func (p *Parser) curTokenIs(t lexer.TokenType) bool {
	return p.curToken.Type == t
}

func (p *Parser) peekTokenIs(t lexer.TokenType) bool {
	return p.peekToken.Type == t
}

// expectPeek checks the type of the next token and advances if it matches.
// If it doesn't match, it adds an error.
func (p *Parser) expectPeek(t lexer.TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	} else {
		p.peekError(t)
		return false
	}
}

// --- Error Handling ---

func (p *Parser) peekError(t lexer.TokenType) {
	msg := fmt.Sprintf("expected next token to be %s, got %s instead",
		t, p.peekToken.Type)
	p.addError(p.peekToken, msg)
}

func (p *Parser) noPrefixParseFnError(t lexer.TokenType) {
	msg := fmt.Sprintf("no prefix parse function for %s found", t)
	p.addError(p.curToken, msg)
}

// --- Precedence Helper ---
func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.Type]; ok {
		return p
	}
	return LOWEST
}

// -- Prefix Parse Functions --

// parsePrefixExpression handles expressions like !expr or -expr
func (p *Parser) parsePrefixExpression() Expression {
	expression := &PrefixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
	}

	p.nextToken() // Consume the operator

	expression.Right = p.parseExpression(PREFIX) // Parse the right-hand side with PREFIX precedence

	return expression
}

// parseTypeofExpression parses a typeof expression.
func (p *Parser) parseTypeofExpression() Expression {
	expression := &TypeofExpression{
		Token: p.curToken, // The 'typeof' token
	}

	p.nextToken() // Move past 'typeof'

	// Parse the operand with PREFIX precedence
	expression.Operand = p.parseExpression(PREFIX)
	if expression.Operand == nil {
		p.addError(p.curToken, "expected expression after 'typeof'")
		return nil
	}

	return expression
}

// parseTypeAssertionExpression handles type assertion expressions like (value as Type)
func (p *Parser) parseTypeAssertionExpression(left Expression) Expression {
	expression := &TypeAssertionExpression{
		Token:      p.curToken, // The 'as' token
		Expression: left,       // The expression being asserted
	}

	p.nextToken() // Move past 'as'

	// Parse the target type expression
	expression.TargetType = p.parseTypeExpression()
	if expression.TargetType == nil {
		p.addError(p.curToken, "expected type after 'as'")
		return nil
	}

	return expression
}

// parseGroupedExpression handles expressions like (expr) OR arrow functions like () => expr or (a, b) => expr
func (p *Parser) parseGroupedExpression() Expression {
	startPos := p.l.CurrentPosition()
	startCur := p.curToken
	startPeek := p.peekToken
	startErrors := len(p.errors)
	debugPrint("parseGroupedExpression: Starting at pos %d, cur='%s', peek='%s'", startPos, startCur.Literal, startPeek.Literal)

	// --- Attempt to parse as Arrow Function Parameters ---
	if p.curTokenIs(lexer.LPAREN) {
		debugPrint("parseGroupedExpression: Attempting arrow param parse...")
		params, restParam, _ := p.parseParameterList() // Consumes up to and including ')'

		// Case 1: Arrow function with params, NO return type annotation: (a, b) => body
		if params != nil && p.curTokenIs(lexer.RPAREN) && p.peekTokenIs(lexer.ARROW) {
			debugPrint("parseGroupedExpression: Successfully parsed arrow params: %v, found '=>' next.", params)
			p.nextToken() // Consume ')', Now curToken is '=>'
			debugPrint("parseGroupedExpression: Consumed ')', cur is now '=>'")
			p.errors = p.errors[:startErrors]                                // Clear errors from backtrack attempt
			return p.parseArrowFunctionBodyAndFinish(params, restParam, nil) // No return type annotation

			// Case 2: Arrow function with params AND return type annotation: (a: T, b: U): R => body
		} else if params != nil && p.curTokenIs(lexer.RPAREN) && p.peekTokenIs(lexer.COLON) {
			debugPrint("parseGroupedExpression: Successfully parsed arrow params: %v, found ':' next.", params)
			p.nextToken() // Consume ':', curToken is now ':'
			p.nextToken() // Consume the token starting the type expression, cur is start of type (e.g., 'number')
			debugPrint("parseGroupedExpression: Consumed ':', cur='%s' (%s)", p.curToken.Literal, p.curToken.Type)
			p.errors = p.errors[:startErrors] // Clear errors from backtrack attempt

			returnTypeAnnotation := p.parseTypeExpression() // Consumes type, cur is last token of type (e.g., 'number')

			if returnTypeAnnotation == nil {
				return nil // Propagate error from type parsing
			}
			// AFTER parseTypeExpression, curToken is the *last* token of the type annotation.
			debugPrint("parseGroupedExpression: Parsed return type annotation %T. cur='%s', peek='%s'", returnTypeAnnotation, p.curToken.Literal, p.peekToken.Literal)

			// Check if the token *after* the type annotation is '=>'
			if !p.peekTokenIs(lexer.ARROW) {
				msg := fmt.Sprintf("expected '=>' after return type annotation, got %s", p.peekToken.Type)
				p.addError(p.peekToken, msg)
				debugPrint("parseGroupedExpression: Error - %s", msg)
				return nil
			}

			// Consume the last token of the type expression (which is curToken).
			// This makes '=>' the new curToken.
			p.nextToken()
			debugPrint("parseGroupedExpression: Consumed type expr end, cur is now '=>'")

			// Pass the correctly parsed returnTypeAnnotation.
			// parseArrowFunctionBodyAndFinish expects curToken to be '=>'.
			return p.parseArrowFunctionBodyAndFinish(params, restParam, returnTypeAnnotation)

		} else {
			// Not an arrow function (or parseParameterList failed), backtrack.
			debugPrint("parseGroupedExpression: Failed arrow param parse (params=%v, cur='%s', peek='%s') or no '=>' or ':', backtracking...", params, p.curToken.Literal, p.peekToken.Type)
			// --- PRECISE BACKTRACK ---
			p.l.SetPosition(startPos) // Reset lexer position
			p.curToken = startCur     // Restore original curToken
			p.peekToken = startPeek   // Restore original peekToken
			p.errors = p.errors[:startErrors]
			debugPrint("parseGroupedExpression: Precise Backtrack complete. cur='%s', peek='%s'", p.curToken.Literal, p.peekToken.Literal)
			// Fall through to parse as regular grouped expression
		}
	} else {
		debugPrint("parseGroupedExpression: Not starting with '(', cannot be parenthesized arrow params.")
		// Fall through to parse as regular grouped expression
	}

	// --- If not arrow function, parse as regular Grouped Expression ---
	debugPrint("parseGroupedExpression: Parsing as regular grouped expression.")
	if !p.curTokenIs(lexer.LPAREN) { // Check curToken IS LPAREN after potential backtrack
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}
	p.nextToken() // Consume '('
	debugPrint("parseGroupedExpression: Consumed '(', cur='%s'", p.curToken.Literal)
	exp := p.parseExpression(LOWEST)
	if exp == nil {
		return nil // Propagate error from inner expression
	}
	if !p.expectPeek(lexer.RPAREN) {
		return nil // Missing closing parenthesis
	}
	debugPrint("parseGroupedExpression: Finished grouped expr %T", exp)
	return exp
}

// parseIfExpression parses an if expression: if (condition) { consequence } else { alternative }
func (p *Parser) parseIfExpression() Expression {
	debugPrint("parseIfExpression starting...")
	expr := &IfExpression{Token: p.curToken} // 'if' token

	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	p.nextToken() // Consume '('
	debugPrint("parseIfExpression parsing condition...")
	expr.Condition = p.parseExpression(LOWEST)
	if expr.Condition == nil {
		return nil
	} // <<< NIL CHECK
	debugPrint("parseIfExpression parsed condition: %s", expr.Condition.String())

	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}

	// --- MODIFIED: Handle both block statements and single statements ---
	if p.peekTokenIs(lexer.LBRACE) {
		// Block statement case: if (condition) { ... }
		if !p.expectPeek(lexer.LBRACE) {
			return nil
		}
		debugPrint("parseIfExpression parsing consequence block...")
		expr.Consequence = p.parseBlockStatement()
	} else {
		// Single statement case: if (condition) statement
		p.nextToken() // Move to the start of the statement
		debugPrint("parseIfExpression parsing single consequence statement...")
		stmt := p.parseStatement()
		if stmt == nil {
			return nil
		}
		// Wrap the single statement in a BlockStatement
		expr.Consequence = &BlockStatement{
			Token:               p.curToken, // Use current token for the wrapper
			Statements:          []Statement{stmt},
			HoistedDeclarations: make(map[string]Expression),
		}
	}
	// --- END MODIFICATION ---

	if expr.Consequence == nil {
		return nil
	} // <<< NIL CHECK
	debugPrint("parseIfExpression parsed consequence.")

	// Check for optional 'else' block
	if p.peekTokenIs(lexer.ELSE) {
		p.nextToken() // Consume 'else'
		debugPrint("parseIfExpression found 'else'...")

		// Allow 'else if' by parsing another IfExpression directly
		if p.peekTokenIs(lexer.IF) {
			debugPrint("parseIfExpression found 'else if'...")
			p.nextToken() // Consume 'if' for the 'else if' case

			// The alternative for an 'else if' is the nested IfExpression itself.
			// However, the AST expects a BlockStatement. We wrap the IfExpression
			// in a dummy BlockStatement.
			elseIfExpr := p.parseIfExpression() // Recursively parse the 'else if'
			if elseIfExpr == nil {
				return nil // Propagate error
			}
			// Wrap the nested IfExpression in a BlockStatement for the Alternative field
			// We use the 'else' token for the block, as it's the start of the alternative branch
			elseBlock := &BlockStatement{Token: expr.Token} // Use the 'else' token?
			elseBlock.Statements = []Statement{&ExpressionStatement{Expression: elseIfExpr}}
			elseBlock.HoistedDeclarations = make(map[string]Expression)
			expr.Alternative = elseBlock
			debugPrint("parseIfExpression parsed 'else if' branch.")

		} else if p.peekTokenIs(lexer.LBRACE) {
			// Block statement case: else { ... }
			if !p.expectPeek(lexer.LBRACE) {
				return nil
			}
			debugPrint("parseIfExpression parsing standard 'else' block...")
			// Call parseBlockStatement first before assigning
			alternativeBlock := p.parseBlockStatement()

			// --- DEBUG: Log state of block BEFORE assignment ---
			debugPrint("// [Parser IfExpr] Assigning Alternative: BlockPtr=%p", alternativeBlock)
			if alternativeBlock != nil {
				statementsPtr := &alternativeBlock.Statements // Get pointer to the slice header
				debugPrint(", StmtSliceHeaderPtr=%p", statementsPtr)
				if alternativeBlock.Statements == nil {
					debugPrint(", Statements=nil\n")
				} else {
					debugPrint(", Statements.Len=%d\n", len(alternativeBlock.Statements))
				}
			} else {
				debugPrint(", Block=nil\n")
			}
			// --- END DEBUG ---

			expr.Alternative = alternativeBlock // Assign the parsed block

			if expr.Alternative == nil {
				return nil
			} // <<< NIL CHECK
			debugPrint("parseIfExpression parsed standard 'else' block.")
		} else {
			// --- NEW: Single statement case: else statement ---
			p.nextToken() // Move to the start of the else statement
			debugPrint("parseIfExpression parsing single 'else' statement...")
			stmt := p.parseStatement()
			if stmt == nil {
				return nil
			}
			// Wrap the single statement in a BlockStatement
			expr.Alternative = &BlockStatement{
				Token:               p.curToken, // Use current token for the wrapper
				Statements:          []Statement{stmt},
				HoistedDeclarations: make(map[string]Expression),
			}
			debugPrint("parseIfExpression parsed single 'else' statement.")
			// --- END NEW ---
		}
	} else {
		debugPrint("parseIfExpression found no 'else' branch.")
	}

	debugPrint("parseIfExpression finished, returning: %s", expr.String())
	return expr
}

// -- Infix Parse Functions --

// parseInfixExpression handles expressions like left op right
func (p *Parser) parseInfixExpression(left Expression) Expression {
	debugPrint("parseInfixExpression: Starting. left=%T('%s'), cur='%s' (%s)", left, left.String(), p.curToken.Literal, p.curToken.Type)
	expression := &InfixExpression{
		Token:    p.curToken, // The operator token
		Operator: p.curToken.Literal,
		Left:     left,
	}

	// --- Associativity Fix ---
	precedence := p.curPrecedence()
	if expression.Token.Type == lexer.EXPONENT { // Check the actual operator token type
		precedence-- // For right-associative **, parse right-hand side with lower precedence
		debugPrint("parseInfixExpression: Right-associative '%s', parsing right with precedence %d", expression.Operator, precedence)
	} else {
		debugPrint("parseInfixExpression: Left-associative '%s', parsing right with precedence %d", expression.Operator, precedence)
	}
	p.nextToken()                                    // Consume the operator
	expression.Right = p.parseExpression(precedence) // Parse the right operand with potentially adjusted precedence

	if expression.Right == nil {
		debugPrint("parseInfixExpression: Right expression was nil, returning nil.")
		return nil // Error occurred parsing right side
	}
	debugPrint("parseInfixExpression: Finished. Right=%T('%s')", expression.Right, expression.Right.String())
	return expression
}

// parseCallExpression handles function calls like func(arg1, arg2)
func (p *Parser) parseCallExpression(function Expression) Expression {
	exp := &CallExpression{Token: p.curToken, Function: function}
	exp.Arguments = p.parseExpressionList(lexer.RPAREN)
	return exp
}

// parseExpressionList parses a comma-separated list of expressions until a specific end token.
func (p *Parser) parseExpressionList(end lexer.TokenType) []Expression {
	list := []Expression{}

	// Check for empty list: call() or []
	if p.peekTokenIs(end) {
		p.nextToken() // Consume the end token (e.g., ')' or ']')
		return list
	}

	p.nextToken() // Consume '(' or '[' to get to the first expression
	expr := p.parseExpression(LOWEST)
	if expr == nil {
		return nil // Propagate error from parsing the first element
	}
	list = append(list, expr)

	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken() // Consume ','

		// --- Allow trailing comma ---
		if p.peekTokenIs(end) {
			p.nextToken() // Consume the end token
			return list
		}
		// --- End Trailing Comma Handling ---

		p.nextToken() // Consume the token starting the next expression
		expr = p.parseExpression(LOWEST)
		if expr == nil {
			return nil // Propagate error from parsing subsequent element
		}
		list = append(list, expr)
	}

	if !p.expectPeek(end) {
		return nil // Expected the end token (e.g., ')' or ']')
	}

	return list
}

// parseArrowFunctionBodyAndFinish completes parsing an arrow function.
// It assumes the parameters have been parsed and the current token is '=>'.
func (p *Parser) parseArrowFunctionBodyAndFinish(params []*Parameter, restParam *RestParameter, returnTypeAnnotation Expression) Expression {
	debugPrint("parseArrowFunctionBodyAndFinish: Starting, curToken='%s' (%s), params=%v, restParam=%v", p.curToken.Literal, p.curToken.Type, params, restParam)
	arrowFunc := &ArrowFunctionLiteral{
		Token:                p.curToken, // The '=>' token
		Parameters:           params,     // Use the passed-in parameters
		RestParameter:        restParam,  // Use the passed-in rest parameter
		ReturnTypeAnnotation: returnTypeAnnotation,
	}

	p.nextToken() // Consume '=>' ONLY
	debugPrint("parseArrowFunctionBodyAndFinish: Consumed '=>', cur='%s' (%s), peek='%s' (%s)", p.curToken.Literal, p.curToken.Type, p.peekToken.Literal, p.peekToken.Type)

	if p.curTokenIs(lexer.LBRACE) {
		debugPrint("parseArrowFunctionBodyAndFinish: Parsing BlockStatement body...")
		arrowFunc.Body = p.parseBlockStatement() // parseBlockStatement consumes { and } internally
	} else {
		debugPrint("parseArrowFunctionBodyAndFinish: Parsing Expression body...")
		// No nextToken here - curToken is already the start of the expression
		arrowFunc.Body = p.parseExpression(LOWEST)
	}
	debugPrint("parseArrowFunctionBodyAndFinish: Finished parsing body=%T, returning ArrowFunc", arrowFunc.Body)
	return arrowFunc
}

// parseParameterList parses a list of identifiers enclosed in parentheses.
// Expects the current token to be '('. Consumes tokens up to and including the closing ')'.
// Returns the list of parameters and optional rest parameter, or nil if parsing fails.
func (p *Parser) parseParameterList() ([]*Parameter, *RestParameter, error) {
	params := []*Parameter{}
	var restParam *RestParameter

	if !p.curTokenIs(lexer.LPAREN) { // Check current token IS LPAREN
		// This case should ideally not be hit if called correctly from parseGroupedExpression
		return nil, nil, fmt.Errorf("expected '('")
	}
	debugPrint("parseParameterList: Starting, cur='%s', peek='%s'", p.curToken.Literal, p.peekToken.Literal)

	// Handle empty list: () => ...
	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken() // Consume ')'
		debugPrint("parseParameterList: Found empty list '()'")
		return params, nil, nil // Return empty slice
	}

	// Parse the first parameter
	p.nextToken() // Move past '(' to the first parameter identifier or spread

	// Check if first parameter is a rest parameter
	if p.curTokenIs(lexer.SPREAD) {
		debugPrint("parseParameterList: Found rest parameter at start")
		restParam = p.parseRestParameter()
		if restParam == nil {
			return nil, nil, fmt.Errorf("failed to parse rest parameter")
		}
		// Rest parameter must be last, so expect closing parenthesis
		if !p.expectPeek(lexer.RPAREN) {
			return nil, nil, fmt.Errorf("expected closing parenthesis after rest parameter")
		}
		debugPrint("parseParameterList: Consumed ')', finished with rest parameter.")
		return params, restParam, nil
	}

	// Parse regular parameter
	if !p.curTokenIs(lexer.IDENT) {
		msg := fmt.Sprintf("expected identifier as parameter, got %s", p.curToken.Type)
		p.addError(p.curToken, msg)
		debugPrint("parseParameterList: Error - %s", msg)
		return nil, nil, fmt.Errorf("%s", msg)
	}
	param := &Parameter{Token: p.curToken}
	param.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check for optional parameter (?)
	if p.peekTokenIs(lexer.QUESTION) {
		p.nextToken() // Consume '?'
		param.Optional = true
	}

	// Check for Type Annotation
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ':'
		p.nextToken() // Consume token starting the type expression
		param.TypeAnnotation = p.parseTypeExpression()
		if param.TypeAnnotation == nil {
			return nil, nil, fmt.Errorf("failed to parse type annotation for parameter")
		} // Propagate error
	} else {
		param.TypeAnnotation = nil
	}

	// Check for Default Value
	if p.peekTokenIs(lexer.ASSIGN) {
		p.nextToken() // Consume '='
		p.nextToken() // Move to expression
		param.DefaultValue = p.parseExpression(LOWEST)
		if param.DefaultValue == nil {
			p.addError(p.curToken, "expected expression after '=' in parameter default value")
			return nil, nil, fmt.Errorf("expected expression after '=' in parameter default value")
		}
	}

	params = append(params, param)
	debugPrint("parseParameterList: Parsed param '%s' (type: %v)", param.Name.Value, param.TypeAnnotation)

	// Parse subsequent parameters (comma-separated)
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken() // Consume ','
		p.nextToken() // Consume identifier or spread

		// Check if this is a rest parameter
		if p.curTokenIs(lexer.SPREAD) {
			debugPrint("parseParameterList: Found rest parameter after comma")
			restParam = p.parseRestParameter()
			if restParam == nil {
				return nil, nil, fmt.Errorf("failed to parse rest parameter")
			}
			// Rest parameter must be last, so expect closing parenthesis
			if !p.expectPeek(lexer.RPAREN) {
				return nil, nil, fmt.Errorf("expected closing parenthesis after rest parameter")
			}
			debugPrint("parseParameterList: Consumed ')', finished with rest parameter.")
			return params, restParam, nil
		}

		// Parse regular parameter
		if !p.curTokenIs(lexer.IDENT) {
			msg := fmt.Sprintf("expected identifier for parameter name after comma, got %s", p.curToken.Type)
			p.addError(p.curToken, msg)
			debugPrint("parseParameterList: Error - %s", msg)
			return nil, nil, fmt.Errorf("%s", msg)
		}
		param := &Parameter{Token: p.curToken}
		param.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

		// Check for optional parameter (?)
		if p.peekTokenIs(lexer.QUESTION) {
			p.nextToken() // Consume '?'
			param.Optional = true
		}

		// Check for Type Annotation
		if p.peekTokenIs(lexer.COLON) {
			p.nextToken() // Consume ':'
			p.nextToken() // Consume token starting the type expression
			param.TypeAnnotation = p.parseTypeExpression()
			if param.TypeAnnotation == nil {
				return nil, nil, fmt.Errorf("failed to parse type annotation for parameter")
			} // Propagate error
		} else {
			param.TypeAnnotation = nil
		}

		// Check for Default Value
		if p.peekTokenIs(lexer.ASSIGN) {
			p.nextToken() // Consume '='
			p.nextToken() // Move to expression
			param.DefaultValue = p.parseExpression(LOWEST)
			if param.DefaultValue == nil {
				p.addError(p.curToken, "expected expression after '=' in parameter default value")
				return nil, nil, fmt.Errorf("expected expression after '=' in parameter default value")
			}
		}

		params = append(params, param)
		debugPrint("parseParameterList: Parsed param '%s' (type: %v)", param.Name.Value, param.TypeAnnotation)
	}

	// Expect closing parenthesis
	if !p.expectPeek(lexer.RPAREN) {
		debugPrint("parseParameterList: Expected ')' after parameters, got peek '%s'", p.peekToken.Type)
		return nil, nil, fmt.Errorf("expected closing parenthesis after parameters")
	}
	debugPrint("parseParameterList: Consumed ')', finished successfully.")

	return params, restParam, nil
}

// parseTernaryExpression parses condition ? consequence : alternative
func (p *Parser) parseTernaryExpression(condition Expression) Expression {
	debugPrint("parseTernaryExpression starting with condition: %s", condition.String())
	expr := &TernaryExpression{
		Token:     p.curToken, // The '?' token
		Condition: condition,
	}

	p.nextToken() // Consume '?'

	// Parse the consequence expression
	debugPrint("parseTernaryExpression parsing consequence...")
	expr.Consequence = p.parseExpression(LOWEST) // Ternary has lowest precedence for right-hand side parts
	if expr.Consequence == nil {
		return nil
	} // <<< NIL CHECK
	debugPrint("parseTernaryExpression parsed consequence: %s", expr.Consequence.String())

	if !p.expectPeek(lexer.COLON) {
		debugPrint("parseTernaryExpression failed: expected COLON")
		return nil // Error already added by expectPeek
	}

	p.nextToken() // Consume ':'

	// Parse the alternative expression
	debugPrint("parseTernaryExpression parsing alternative...")
	expr.Alternative = p.parseExpression(LOWEST) // Continue with low precedence
	if expr.Alternative == nil {
		return nil
	} // <<< NIL CHECK
	debugPrint("parseTernaryExpression parsed alternative: %s", expr.Alternative.String())

	debugPrint("parseTernaryExpression finished, returning: %s", expr.String())
	return expr
}

// parseAssignmentExpression handles variable assignment (e.g., x = value)
func (p *Parser) parseAssignmentExpression(left Expression) Expression {
	debugPrint("parseAssignmentExpression starting with left: %s (%T)", left.String(), left)
	expr := &AssignmentExpression{
		Token:    p.curToken,         // The assignment token (=, +=, etc.)
		Operator: p.curToken.Literal, // Store the operator string
		Left:     left,
	}

	// Check if the left side is assignable using the shared utility function
	if !p.isValidLValue(left) {
		msg := fmt.Sprintf("invalid left-hand side in assignment: %s", left.String())
		p.addError(expr.Token, msg)
		return nil
	}

	precedence := p.curPrecedence()
	p.nextToken() // Consume assignment operator

	debugPrint("parseAssignmentExpression parsing right side...")
	expr.Value = p.parseExpression(precedence)
	debugPrint("parseAssignmentExpression finished right side: %s (%T)", expr.Value.String(), expr.Value)

	return expr
}

// --- New: While Statement Parsing ---

func (p *Parser) parseWhileStatement() *WhileStatement {
	// Parses 'while' '(' <condition> ')' <block_statement>
	stmt := &WhileStatement{Token: p.curToken} // Current token is 'while'

	if !p.expectPeek(lexer.LPAREN) {
		return nil // Expected '(' after 'while'
	}

	p.nextToken() // Consume '('
	stmt.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.RPAREN) {
		return nil // Expected ')' after condition
	}

	// --- MODIFIED: Handle both block statements and single statements ---
	if p.peekTokenIs(lexer.LBRACE) {
		// Block statement case: while (condition) { ... }
		if !p.expectPeek(lexer.LBRACE) {
			return nil
		}
		stmt.Body = p.parseBlockStatement()
	} else {
		// Single statement case: while (condition) statement
		p.nextToken() // Move to the start of the statement
		bodyStmt := p.parseStatement()
		if bodyStmt == nil {
			return nil
		}
		// Wrap the single statement in a BlockStatement
		stmt.Body = &BlockStatement{
			Token:               p.curToken,
			Statements:          []Statement{bodyStmt},
			HoistedDeclarations: make(map[string]Expression),
		}
	}
	// --- END MODIFICATION ---

	return stmt
}

// --- New: For Statement Parsing ---

func (p *Parser) parseForStatement() Statement {
	debugPrint("parseForStatement: START, cur='%s'", p.curToken.Literal)

	// Parse the opening structure first
	forToken := p.curToken

	if !p.expectPeek(lexer.LPAREN) {
		debugPrint("parseForStatement: ERROR expected LPAREN")
		return nil
	}

	if !p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken() // Move past '('

		// Try to detect for...of pattern
		if p.curTokenIs(lexer.LET) || p.curTokenIs(lexer.CONST) || p.curTokenIs(lexer.IDENT) {
			// Check if this could be for...of by looking for pattern: variable of expression
			return p.parseForStatementOrForOf(forToken)
		}
	}

	// If we get here, it's a regular for loop
	return p.parseRegularForStatement(forToken)
}

// --- New: Break/Continue Statement Parsing ---

func (p *Parser) parseBreakStatement() *BreakStatement {
	stmt := &BreakStatement{Token: p.curToken} // Current token is 'break'

	// Consume optional semicolon
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseContinueStatement() *ContinueStatement {
	stmt := &ContinueStatement{Token: p.curToken} // Current token is 'continue'

	// Consume optional semicolon
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

// --- New: Do-While Statement Parsing ---

func (p *Parser) parseDoWhileStatement() *DoWhileStatement {
	stmt := &DoWhileStatement{Token: p.curToken}

	// --- MODIFIED: Handle both block statements and single statements ---
	if p.peekTokenIs(lexer.LBRACE) {
		// Block statement case: do { ... } while (condition)
		if !p.expectPeek(lexer.LBRACE) {
			return nil
		}
		stmt.Body = p.parseBlockStatement()
	} else {
		// Single statement case: do statement while (condition)
		p.nextToken() // Move to the start of the statement
		bodyStmt := p.parseStatement()
		if bodyStmt == nil {
			return nil
		}
		// Wrap the single statement in a BlockStatement
		stmt.Body = &BlockStatement{
			Token:               p.curToken,
			Statements:          []Statement{bodyStmt},
			HoistedDeclarations: make(map[string]Expression),
		}
	}
	// --- END MODIFICATION ---

	// Expect 'while' after the block
	if !p.expectPeek(lexer.WHILE) {
		return nil
	}

	// Expect '(' after 'while'
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	p.nextToken() // Consume '(', move to expression
	stmt.Condition = p.parseExpression(LOWEST)

	// Expect ')' after expression
	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}

	// Optional semicolon
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

// --- New: Update Expression Parsing ---

// isValidLValue checks if an expression can be used as an lvalue (left-hand side of assignment or update operations)
func (p *Parser) isValidLValue(expr Expression) bool {
	switch expr.(type) {
	case *Identifier:
		return true
	case *IndexExpression:
		return true
	case *MemberExpression:
		return true
	default:
		return false
	}
}

func (p *Parser) parsePrefixUpdateExpression() Expression {
	expr := &UpdateExpression{
		Token:    p.curToken, // ++ or --
		Operator: p.curToken.Literal,
		Prefix:   true,
	}
	p.nextToken()                             // Consume ++ or --
	expr.Argument = p.parseExpression(PREFIX) // Parse argument with PREFIX precedence

	// Check if argument is assignable (Identifier, IndexExpression, or MemberExpression)
	if !p.isValidLValue(expr.Argument) {
		msg := fmt.Sprintf("invalid argument for prefix %s: expected identifier, member expression, or index expression, got %T",
			expr.Operator, expr.Argument)
		p.addError(expr.Token, msg)
		return nil
	}

	return expr
}

func (p *Parser) parsePostfixUpdateExpression(left Expression) Expression {
	expr := &UpdateExpression{
		Token:    p.curToken, // ++ or --
		Operator: p.curToken.Literal,
		Argument: left, // Argument is the expression on the left
		Prefix:   false,
	}

	// Check if argument is assignable (Identifier, IndexExpression, or MemberExpression)
	if !p.isValidLValue(expr.Argument) {
		msg := fmt.Sprintf("invalid argument for postfix %s: expected identifier, member expression, or index expression, got %T",
			expr.Operator, expr.Argument)
		p.addError(expr.Token, msg)
		return nil
	}

	// No need to consume token, parseExpression loop does that.
	return expr
}

// --- NEW: Array Literal Parsing ---
func (p *Parser) parseArrayLiteral() Expression {
	array := &ArrayLiteral{Token: p.curToken} // '['

	array.Elements = p.parseExpressionList(lexer.RBRACKET)
	if array.Elements == nil {
		// If parseExpressionList returned nil, it means it didn't find the RBRACKET.
		// Error message was likely added by expectPeek within parseExpressionList.
		return nil
	}

	return array
}

// --- NEW: Index Expression Parsing ---
func (p *Parser) parseIndexExpression(left Expression) Expression {
	exp := &IndexExpression{
		Token: p.curToken, // '['
		Left:  left,
	}

	p.nextToken() // Consume '[', move to the start of the index expression
	exp.Index = p.parseExpression(LOWEST)
	if exp.Index == nil {
		return nil // Error parsing index expression
	}

	if !p.expectPeek(lexer.RBRACKET) {
		return nil // Expected ']'
	}

	return exp
}

// --- NEW: parseMemberExpression function ---
func (p *Parser) parseMemberExpression(left Expression) Expression {
	// Current token should be DOT
	exp := &MemberExpression{
		Token:  p.curToken, // The '.' token
		Object: left,
	}

	// Set precedence for parsing the property identifier
	// Member access has higher precedence than most operators

	// Move to the next token (which should be the property name)
	p.nextToken()
	
	// Parse property name (allowing keywords as property names)
	propIdent := p.parsePropertyName()
	if propIdent == nil {
		// If the token after '.' is not a valid property name, it's a syntax error.
		msg := fmt.Sprintf("expected identifier after '.', got %s", p.curToken.Type)
		p.addError(p.curToken, msg)
		return nil
	}

	exp.Property = propIdent

	// We don't call parseExpression here because the right side MUST be an identifier.
	// The precedence check in the main parseExpression loop handles chaining, e.g., a.b.c
	return exp
}

// addError creates a SyntaxError and appends it to the parser's error list.
func (p *Parser) addError(tok lexer.Token, msg string) {
	syntaxErr := &errors.SyntaxError{
		Position: errors.Position{
			Line:     tok.Line,
			Column:   tok.Column,
			StartPos: tok.StartPos,
			EndPos:   tok.EndPos,
		},
		Msg: msg,
	}
	p.errors = append(p.errors, syntaxErr)
}

// --- NEW: Switch Statement Parsing ---

// parseSwitchStatement parses a switch statement:
// switch ( <expression> ) { <caseClauses> }
func (p *Parser) parseSwitchStatement() *SwitchStatement {
	stmt := &SwitchStatement{Token: p.curToken} // 'switch' token
	stmt.Cases = []*SwitchCase{}

	if !p.expectPeek(lexer.LPAREN) {
		return nil // Expected '(' after 'switch'
	}

	p.nextToken() // Consume '('
	stmt.Expression = p.parseExpression(LOWEST)
	if stmt.Expression == nil {
		return nil // Error parsing switch expression
	}

	if !p.expectPeek(lexer.RPAREN) {
		return nil // Expected ')' after switch expression
	}

	if !p.expectPeek(lexer.LBRACE) {
		return nil // Expected '{' to start switch body
	}

	p.nextToken() // Consume '{'

	// Parse case/default clauses
	for !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
		if p.curTokenIs(lexer.CASE) || p.curTokenIs(lexer.DEFAULT) {
			caseClause := p.parseSwitchCase()
			if caseClause != nil {
				stmt.Cases = append(stmt.Cases, caseClause)
			} else {
				// Error parsing case, try to recover by advancing until next potential case/default/end
				p.nextToken() // Consume the token that caused the error in parseSwitchCase
				for !p.curTokenIs(lexer.CASE) && !p.curTokenIs(lexer.DEFAULT) && !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
					p.nextToken()
				}
				continue // Continue parsing the next case/default if found
			}
			// parseSwitchCase leaves the current token at the start of the *next* case/default or RBRACE
		} else {
			msg := fmt.Sprintf("expected 'case' or 'default' inside switch block, got %s instead", p.curToken.Type)
			p.addError(p.curToken, msg)
			// Recovery: Advance until we potentially find the next clause or the end brace
			for !p.curTokenIs(lexer.CASE) && !p.curTokenIs(lexer.DEFAULT) && !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
				p.nextToken()
			}
		}
		// Do not call nextToken() here, parseSwitchCase or the error recovery loop should leave curToken ready for the next iteration check
	}

	if !p.curTokenIs(lexer.RBRACE) {
		p.peekError(lexer.RBRACE) // Expected '}'
		return nil
	}

	// Don't consume '}' here, let the main ParseProgram loop advance

	return stmt
}

// parseSwitchCase parses a single 'case' or 'default' clause within a switch statement.
func (p *Parser) parseSwitchCase() *SwitchCase {
	caseClause := &SwitchCase{Token: p.curToken} // 'case' or 'default' token

	if p.curTokenIs(lexer.CASE) {
		p.nextToken() // Consume 'case'
		caseClause.Condition = p.parseExpression(LOWEST)
		if caseClause.Condition == nil {
			return nil // Error parsing case condition
		}
		// After parseExpression, curToken is the last token of the expression.
		// We expect the *next* token (peek) to be ':'.
		if !p.peekTokenIs(lexer.COLON) { // Check if peek is colon
			p.peekError(lexer.COLON)
			return nil // Expected ':' after case expression
		}
		// Colon is present in peek. Advance twice: once past expr end, once past colon.
		p.nextToken() // Consume end-of-expression token
		p.nextToken() // Consume ':'
	} else { // Must be DEFAULT
		p.nextToken() // Consume 'default'
		// Now curToken *should* be the ':'.
		if !p.curTokenIs(lexer.COLON) { // Check the CURRENT token
			p.peekError(lexer.COLON) // Report error based on expectation
			return nil               // Expected ':' immediately after 'default'
		}
		// curToken is ':', condition is nil implicitly.
		p.nextToken() // Consume ':' once
	}

	// Now curToken is the first token of the statement list after the colon.

	// Parse the statements belonging to this case
	caseClause.Body = &BlockStatement{Token: caseClause.Token}
	caseClause.Body.Statements = []Statement{}

	// Loop until the next case, default, or the end of the switch block
	// Similar loop logic as parseBlockStatement
	for !p.curTokenIs(lexer.CASE) && !p.curTokenIs(lexer.DEFAULT) && !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
		stmt := p.parseStatement() // parseStatement consumes tokens including optional semicolon
		if stmt != nil {
			caseClause.Body.Statements = append(caseClause.Body.Statements, stmt)
		} else {
			// If parseStatement returns nil due to an error, break the inner loop
			// to avoid infinite loops and let the outer switch parser handle recovery.
			// An error message should have already been added by parseStatement or its children.
			break
		}

		// Advance AFTER parsing the statement, similar to parseBlockStatement
		// Check for termination conditions before advancing.
		if p.curTokenIs(lexer.EOF) || p.curTokenIs(lexer.CASE) || p.curTokenIs(lexer.DEFAULT) || p.curTokenIs(lexer.RBRACE) {
			break // Reached end of case block or EOF
		}
		p.nextToken() // Advance to the next token to continue parsing statements within the case
	}

	// The token that terminated the loop (CASE, DEFAULT, RBRACE, or EOF) is the current token.
	// We leave it for the outer loop (parseSwitchStatement) to handle.
	return caseClause
}

// --- NEW: parseTypeIdentifier used for simple type names ---
// This function ONLY parses an identifier and returns it. It does not check for '=>'.
func (p *Parser) parseTypeIdentifier() Expression {
	debugPrint("parseTypeIdentifier: cur='%s'", p.curToken.Literal)
	if !p.curTokenIs(lexer.IDENT) {
		// Should not happen if registered correctly
		msg := fmt.Sprintf("internal error: parseTypeIdentifier called on non-IDENT token %s", p.curToken.Type)
		p.addError(p.curToken, msg)
		return nil
	}
	return &Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseObjectLiteral() Expression {
	objLit := &ObjectLiteral{
		Token: p.curToken, // The '{' token
		// --- MODIFIED: Initialize slice ---
		Properties: []*ObjectProperty{},
	}

	for !p.peekTokenIs(lexer.RBRACE) && !p.peekTokenIs(lexer.EOF) {
		p.nextToken() // Consume '{' or ',' to get to the key

		// --- NEW: Check for shorthand method syntax (identifier/keyword followed by '(') ---
		propName := p.parsePropertyName()
		if propName != nil && p.peekTokenIs(lexer.LPAREN) {
			// This is a shorthand method like methodName() { ... }
			shorthandMethod := p.parseShorthandMethod()
			if shorthandMethod == nil {
				return nil // Error parsing shorthand method
			}

			// Create an ObjectProperty with the method name as key and the shorthand method as value
			methodName := shorthandMethod.Name
			objLit.Properties = append(objLit.Properties, &ObjectProperty{Key: methodName, Value: shorthandMethod})
		} else if propName != nil && (p.peekTokenIs(lexer.COMMA) || p.peekTokenIs(lexer.RBRACE)) {
			// --- NEW: Check for shorthand property syntax (identifier/keyword followed by ',' or '}') ---
			// This is shorthand like { name, age } equivalent to { name: name, age: age }
			identName := p.curToken.Literal
			key := propName

			// For shorthand property, the value is also the same identifier
			value := &Identifier{Token: p.curToken, Value: identName}

			// Append the property
			objLit.Properties = append(objLit.Properties, &ObjectProperty{Key: key, Value: value})
		} else {
			// Regular property parsing
			var key Expression
			// --- MODIFIED: Handle Keys (Identifier/Keywords, String, NUMBER, Computed) ---
			if propName != nil {
				key = propName
			} else if p.curTokenIs(lexer.STRING) {
				key = p.parseStringLiteral()
			} else if p.curTokenIs(lexer.NUMBER) { // <<< ADD NUMBER CASE
				key = p.parseNumberLiteral()
			} else if p.curTokenIs(lexer.LBRACKET) { // Computed properties
				p.nextToken() // Consume '['
				key = p.parseExpression(LOWEST)
				if key == nil {
					return nil // Error parsing expression inside []
				}
				if !p.expectPeek(lexer.RBRACKET) {
					return nil // Missing closing ']'
				}
				// After expectPeek, curToken is RBRACKET. parseExpression below needs the next token.
				// We need to be careful here, as the COLON is expected *next*.
			} else {
				// <<< UPDATE ERROR MESSAGE >>>
				msg := fmt.Sprintf("invalid object literal key: expected identifier, string, number, or '[', got %s", p.curToken.Type)
				p.addError(p.curToken, msg)
				return nil
			}
			// --- END MODIFICATION ---

			if key == nil {
				// Error should have been added by the respective parse function
				return nil
			} // Error parsing key

			// Check for Colon *after* parsing the key (including potential closing ']')
			if !p.expectPeek(lexer.COLON) {
				return nil // Expected ':'
			}
			// p.curToken is now COLON

			p.nextToken() // Consume ':' to get to the start of the value

			value := p.parseExpression(LOWEST)
			if value == nil {
				return nil
			} // Error parsing value

			// Append the property
			objLit.Properties = append(objLit.Properties, &ObjectProperty{Key: key, Value: value})
		}

		// Expect ',' or '}'
		if !p.peekTokenIs(lexer.RBRACE) && !p.peekTokenIs(lexer.COMMA) {
			msg := fmt.Sprintf("expected ',' or '}' after object property value, got %s", p.peekToken.Type)
			p.addError(p.peekToken, msg)
			return nil
		}

		if p.peekTokenIs(lexer.COMMA) {
			p.nextToken() // Consume ','
			if p.peekTokenIs(lexer.RBRACE) {
				break // Allow trailing comma
			}
			// If not RBRACE after comma, loop will call nextToken again
		}
	}

	if !p.expectPeek(lexer.RBRACE) {
		return nil
	} // Missing '}'

	return objLit
}

// parseShorthandMethod parses a shorthand method like methodName() { ... }
func (p *Parser) parseShorthandMethod() *ShorthandMethod {
	methodName := p.parsePropertyName()
	if methodName == nil {
		p.addError(p.curToken, "expected method name (identifier) for shorthand method")
		return nil
	}

	method := &ShorthandMethod{
		Token: p.curToken, // The method name token
		Name:  methodName,
	}

	// Expect '(' for parameters
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	// Parse parameters
	method.Parameters, method.RestParameter, _ = p.parseFunctionParameters()
	if method.Parameters == nil && method.RestParameter == nil {
		return nil // Error parsing parameters
	}

	// Check for optional return type annotation
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ')'
		p.nextToken() // Consume ':'
		method.ReturnTypeAnnotation = p.parseTypeExpression()
		if method.ReturnTypeAnnotation == nil {
			return nil // Error parsing return type
		}
	}

	// Expect '{' for method body
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	// Parse method body
	method.Body = p.parseBlockStatement()
	if method.Body == nil {
		return nil // Error parsing method body
	}

	return method
}

// --- NEW: Interface Declaration Parsing ---
func (p *Parser) parseInterfaceDeclaration() *InterfaceDeclaration {
	stmt := &InterfaceDeclaration{Token: p.curToken} // 'interface' token

	if !p.expectPeek(lexer.IDENT) {
		return nil // Expected identifier after 'interface'
	}

	stmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check for extends clause
	if p.peekTokenIs(lexer.EXTENDS) {
		p.nextToken() // Consume 'extends'

		// Parse list of extended interfaces
		for {
			if !p.expectPeek(lexer.IDENT) {
				return nil // Expected interface name after 'extends'
			}

			extendedInterface := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
			stmt.Extends = append(stmt.Extends, extendedInterface)

			// Check for comma to continue list, or break if not found
			if p.peekTokenIs(lexer.COMMA) {
				p.nextToken() // Consume ','
				continue
			} else {
				break
			}
		}
	}

	if !p.expectPeek(lexer.LBRACE) {
		return nil // Expected '{' after interface name or extends clause
	}

	// Parse interface body
	stmt.Properties = []*InterfaceProperty{}

	for !p.peekTokenIs(lexer.RBRACE) && !p.peekTokenIs(lexer.EOF) {
		p.nextToken() // Move to next property

		if p.curTokenIs(lexer.RBRACE) || p.curTokenIs(lexer.EOF) {
			break
		}

		prop := p.parseInterfaceProperty()
		if prop != nil {
			stmt.Properties = append(stmt.Properties, prop)
		}

		// Skip optional semicolon
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken()
		}
	}

	if !p.expectPeek(lexer.RBRACE) {
		return nil // Expected '}' after interface body
	}

	return stmt
}

// parseInterfaceProperty parses a single property in an interface
func (p *Parser) parseInterfaceProperty() *InterfaceProperty {
	// Check for constructor signature first: `new (): T`
	if p.curTokenIs(lexer.NEW) {
		prop := &InterfaceProperty{
			IsConstructorSignature: true,
		}

		// Parse interface constructor signature (uses ':' syntax)
		constructorType := p.parseInterfaceConstructorSignature()
		if constructorType == nil {
			return nil // Error parsing constructor signature
		}

		prop.Type = constructorType
		return prop
	}

	// Check for call signature first: `(): T`
	if p.curTokenIs(lexer.LPAREN) {
		// This is a call signature: (param: type, ...): returnType
		prop := &InterfaceProperty{
			// No name for call signatures
		}

		// Parse method type signature (interfaces use ':' syntax, not '=>')
		funcType := p.parseMethodTypeSignature()
		if funcType == nil {
			return nil // Error parsing method type
		}

		prop.Type = funcType
		return prop
	}

	// Check for shorthand method syntax first (identifier or keyword as property name)
	propName := p.parsePropertyName()
	if propName == nil {
		p.addError(p.curToken, "expected property name (identifier) or call signature '(' in interface")
		return nil
	}

	prop := &InterfaceProperty{
		Name: propName,
	}

	// Check for optional marker '?' first
	if p.peekTokenIs(lexer.QUESTION) {
		p.nextToken() // Consume '?'
		prop.Optional = true
	}

	// Check if this is a shorthand method signature
	if p.peekTokenIs(lexer.LPAREN) {
		// This is a shorthand method signature like methodName(): ReturnType or methodName?(): ReturnType
		p.nextToken() // Move to '('

		// Parse method type signature (uses ':' syntax, not '=>')
		funcType := p.parseMethodTypeSignature()
		if funcType == nil {
			return nil // Error parsing method type
		}

		prop.Type = funcType
		prop.IsMethod = true

		return prop
	}

	// Regular property: PropertyName : TypeExpression
	// Expect ':'
	if !p.expectPeek(lexer.COLON) {
		return nil // Error message already added by expectPeek
	}

	// Parse the type expression
	p.nextToken() // Move to the start of the type expression
	prop.Type = p.parseTypeExpression()
	if prop.Type == nil {
		// Error should have been added by parseTypeExpression
		return nil
	}

	return prop
}

// parseConstructorTypeExpression parses constructor type signatures like `new (): T`
func (p *Parser) parseConstructorTypeExpression() Expression {
	cte := &ConstructorTypeExpression{
		Token: p.curToken, // The 'new' token
	}

	// Expect '(' for parameters
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	// Parse parameter types (similar to function type parameters)
	params, _, err := p.parseFunctionTypeParameterList()
	if err != nil {
		p.addError(p.curToken, err.Error())
		return nil
	}
	cte.Parameters = params
	// Note: Constructor types don't typically use rest parameters, but we parse them anyway

	// Expect '=>' for return type (constructor types use arrow syntax)
	if !p.expectPeek(lexer.ARROW) {
		return nil
	}

	// Parse the constructed type
	p.nextToken() // Move to the start of the return type expression
	cte.ReturnType = p.parseTypeExpression()
	if cte.ReturnType == nil {
		return nil // Error should have been added by parseTypeExpression
	}

	return cte
}

// parseInterfaceConstructorSignature parses constructor signatures in interfaces like `new (): T`
// This is different from parseConstructorTypeExpression which uses arrow syntax for type aliases
func (p *Parser) parseInterfaceConstructorSignature() Expression {
	cte := &ConstructorTypeExpression{
		Token: p.curToken, // The 'new' token
	}

	// Expect '(' for parameters
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	// Parse parameter types (similar to function type parameters)
	params, _, err := p.parseFunctionTypeParameterList()
	if err != nil {
		p.addError(p.curToken, err.Error())
		return nil
	}
	cte.Parameters = params
	// Note: Constructor types don't typically use rest parameters, but we parse them anyway

	// Expect ':' for return type (interface constructor signatures use colon syntax)
	if !p.expectPeek(lexer.COLON) {
		return nil
	}

	// Parse the constructed type
	p.nextToken() // Move to the start of the return type expression
	cte.ReturnType = p.parseTypeExpression()
	if cte.ReturnType == nil {
		return nil // Error should have been added by parseTypeExpression
	}

	return cte
}

// parseObjectTypeExpression parses object type literals like { name: string; age: number }.
func (p *Parser) parseObjectTypeExpression() Expression {
	objType := &ObjectTypeExpression{
		Token:      p.curToken, // The '{' token
		Properties: []*ObjectTypeProperty{},
	}

	// Handle empty object type {}
	if p.peekTokenIs(lexer.RBRACE) {
		p.nextToken() // Consume '}'
		return objType
	}

	// Parse properties
	for !p.peekTokenIs(lexer.RBRACE) && !p.peekTokenIs(lexer.EOF) {
		p.nextToken() // Consume '{' or ';' to get to the property name or call signature

		// Check if this is a call signature starting with '('
		if p.curTokenIs(lexer.LPAREN) {
			// This is a call signature: (param: type, ...): returnType
			prop := &ObjectTypeProperty{
				IsCallSignature: true,
			}

			// Parse parameter types
			params, _, err := p.parseFunctionTypeParameterList()
			if err != nil {
				p.addError(p.curToken, err.Error())
				return nil
			}
			prop.Parameters = params
			// Note: Call signatures in object types don't typically use rest parameters, but we parse them anyway

			// Expect ':' for return type
			if !p.expectPeek(lexer.COLON) {
				return nil
			}

			// Parse the return type
			p.nextToken() // Move to the start of the return type expression
			prop.ReturnType = p.parseTypeExpression()
			if prop.ReturnType == nil {
				return nil // Error should have been added by parseTypeExpression
			}

			objType.Properties = append(objType.Properties, prop)
		} else {
			// Regular property or method signature - try to parse property name (allowing keywords)
			propName := p.parsePropertyName()
			if propName == nil {
				p.addError(p.curToken, "expected property name (identifier) or call signature '(' in object type")
				return nil
			}
			
			prop := &ObjectTypeProperty{
				Name: propName,
			}

			// Check for optional marker '?' first
			if p.peekTokenIs(lexer.QUESTION) {
				p.nextToken() // Consume '?'
				prop.Optional = true
			}

			// Check for shorthand method syntax (identifier followed by '(')
			if p.peekTokenIs(lexer.LPAREN) {
				// This is a shorthand method signature like methodName(): ReturnType or methodName?(): ReturnType
				p.nextToken() // Move to '('

				// Parse method type signature (uses ':' syntax, not '=>')
				funcType := p.parseMethodTypeSignature()
				if funcType == nil {
					return nil // Error parsing method type
				}

				prop.Type = funcType
			} else {
				// Regular property: PropertyName?: TypeExpression

				// Expect ':'
				if !p.expectPeek(lexer.COLON) {
					return nil // Error message already added by expectPeek
				}

				// Parse the type expression
				p.nextToken() // Move to the start of the type expression
				prop.Type = p.parseTypeExpression()
				if prop.Type == nil {
					// Error should have been added by parseTypeExpression
					return nil
				}
			}

			objType.Properties = append(objType.Properties, prop)
		}

		// Expect ';' or '}' next
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken() // Consume ';'
		} else if p.peekTokenIs(lexer.RBRACE) {
			// End of object type, will be consumed by outer loop condition
			break
		} else {
			p.addError(p.peekToken, "expected ';' or '}' after object type property")
			return nil
		}
	}

	// Expect closing '}'
	if !p.expectPeek(lexer.RBRACE) {
		return nil // Error message already added by expectPeek
	}

	return objType
}

// parsePropertyName parses a property name, allowing keywords to be used as identifiers
func (p *Parser) parsePropertyName() *Identifier {
	// Keywords that can be used as property names
	switch p.curToken.Type {
	case lexer.IDENT:
		return &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	case lexer.DELETE, lexer.IF, lexer.ELSE, lexer.FOR, lexer.WHILE, lexer.FUNCTION, 
		 lexer.RETURN, lexer.LET, lexer.CONST, lexer.TRUE, lexer.FALSE, lexer.NULL, 
		 lexer.UNDEFINED, lexer.THIS, lexer.NEW, lexer.TYPEOF, lexer.VOID, lexer.AS, 
		 lexer.IN, lexer.INSTANCEOF:
		// Allow keywords as property names
		return &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	default:
		return nil
	}
}

// parseVoidExpression parses a void expression.
func (p *Parser) parseVoidExpression() Expression {
	expression := &PrefixExpression{
		Token:    p.curToken, // The 'void' token
		Operator: "void",
	}

	p.nextToken() // Move past 'void'

	// Parse the operand with PREFIX precedence
	expression.Right = p.parseExpression(PREFIX)
	if expression.Right == nil {
		p.addError(p.curToken, "expected expression after 'void'")
		return nil
	}

	return expression
}

// parseVoidTypeLiteral parses 'void' as a type annotation.
func (p *Parser) parseVoidTypeLiteral() Expression {
	return &Identifier{Token: p.curToken, Value: "void"}
}

// --- NEW: Try to parse a function overload group ---
func (p *Parser) tryParseFunctionOverloadGroup() *FunctionOverloadGroup {
	// Save parser state in case we need to backtrack
	originalCurToken := p.curToken
	originalPeekToken := p.peekToken
	originalErrors := len(p.errors)

	var overloads []*FunctionSignature
	var functionName string
	var firstToken lexer.Token

	// Try to parse function signatures
	for p.curToken.Type == lexer.FUNCTION {
		// Look ahead to see if this looks like a signature (no body)
		if !p.isLikelyFunctionSignature() {
			// This looks like a function implementation, not a signature
			break
		}

		sig := p.parseFunctionSignature()
		if sig == nil {
			// Failed to parse signature, restore state and return nil
			p.curToken = originalCurToken
			p.peekToken = originalPeekToken
			p.errors = p.errors[:originalErrors] // Remove any errors we added
			return nil
		}

		if len(overloads) == 0 {
			// First signature
			functionName = sig.Name.Value
			firstToken = sig.Token
		} else {
			// Check that the name matches previous signatures
			if sig.Name.Value != functionName {
				// Different function name, this is not part of the overload group
				// Put back the current function declaration for later parsing
				break
			}
		}

		overloads = append(overloads, sig)

		// Move to next statement
		if p.curToken.Type != lexer.EOF {
			p.nextToken()
		}
	}

	// If we didn't find any overload signatures, this isn't an overload group
	if len(overloads) == 0 {
		p.curToken = originalCurToken
		p.peekToken = originalPeekToken
		p.errors = p.errors[:originalErrors]
		return nil
	}

	// Now we should have a function implementation
	if p.curToken.Type != lexer.FUNCTION {
		// No implementation found, restore state
		p.curToken = originalCurToken
		p.peekToken = originalPeekToken
		p.errors = p.errors[:originalErrors]
		return nil
	}

	// Parse the implementation as a function literal
	funcLitExpr := p.parseFunctionLiteral()
	if funcLitExpr == nil {
		// Failed to parse implementation
		p.curToken = originalCurToken
		p.peekToken = originalPeekToken
		p.errors = p.errors[:originalErrors]
		return nil
	}

	funcLit, ok := funcLitExpr.(*FunctionLiteral)
	if !ok {
		// Unexpected type
		p.curToken = originalCurToken
		p.peekToken = originalPeekToken
		p.errors = p.errors[:originalErrors]
		return nil
	}

	// Check that implementation name matches overload signatures
	if funcLit.Name == nil || funcLit.Name.Value != functionName {
		msg := fmt.Sprintf("function implementation name '%s' does not match overload signatures '%s'",
			funcLit.Name.Value, functionName)
		p.addError(funcLit.Name.Token, msg)
		return nil
	}

	// Create the overload group
	group := &FunctionOverloadGroup{
		Token:          firstToken,
		Name:           &Identifier{Token: firstToken, Value: functionName},
		Overloads:      overloads,
		Implementation: funcLit,
	}

	return group
}

// --- NEW: Helper to determine if current function declaration looks like a signature ---
func (p *Parser) isLikelyFunctionSignature() bool {
	// Save current state
	savedCurToken := p.curToken
	savedPeekToken := p.peekToken

	debugPrint("isLikelyFunctionSignature: START cur='%s' peek='%s'", p.curToken.Literal, p.peekToken.Literal)

	// Skip past 'function'
	if p.curToken.Type != lexer.FUNCTION {
		debugPrint("isLikelyFunctionSignature: not a function token")
		return false
	}
	p.nextToken()
	debugPrint("isLikelyFunctionSignature: after function, cur='%s' peek='%s'", p.curToken.Literal, p.peekToken.Literal)

	// Skip past function name (if present)
	if p.curToken.Type == lexer.IDENT {
		p.nextToken()
		debugPrint("isLikelyFunctionSignature: after name, cur='%s' peek='%s'", p.curToken.Literal, p.peekToken.Literal)
	}

	// Skip past parameter list
	if p.curToken.Type == lexer.LPAREN {
		parenCount := 1
		p.nextToken()
		for parenCount > 0 && p.curToken.Type != lexer.EOF {
			if p.curToken.Type == lexer.LPAREN {
				parenCount++
			} else if p.curToken.Type == lexer.RPAREN {
				parenCount--
			}
			p.nextToken()
		}
		debugPrint("isLikelyFunctionSignature: after params, cur='%s' peek='%s'", p.curToken.Literal, p.peekToken.Literal)
	}

	// Skip past return type annotation if present
	if p.curToken.Type == lexer.COLON {
		p.nextToken()
		// Skip the type expression (simplified - just skip until semicolon or brace)
		for p.curToken.Type != lexer.SEMICOLON && p.curToken.Type != lexer.LBRACE && p.curToken.Type != lexer.EOF {
			p.nextToken()
		}
		debugPrint("isLikelyFunctionSignature: after return type, cur='%s' peek='%s'", p.curToken.Literal, p.peekToken.Literal)
	}

	// Check what comes next
	isSignature := p.curToken.Type == lexer.SEMICOLON

	debugPrint("isLikelyFunctionSignature: final decision: %t (cur='%s')", isSignature, p.curToken.Literal)

	// Restore state
	p.curToken = savedCurToken
	p.peekToken = savedPeekToken

	return isSignature
}

// parseOptionalChainingExpression handles optional chaining property access (e.g., obj?.prop)
func (p *Parser) parseOptionalChainingExpression(left Expression) Expression {
	// Current token should be OPTIONAL_CHAINING (?.)
	exp := &OptionalChainingExpression{
		Token:  p.curToken, // The '?.' token
		Object: left,
	}

	if !p.expectPeek(lexer.IDENT) {
		// If the token after '?.' is not an identifier, it's a syntax error.
		msg := fmt.Sprintf("expected identifier after '?.', got %s", p.peekToken.Type)
		p.addError(p.peekToken, msg)
		return nil
	}

	// Construct the Identifier node for the property
	propIdent := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	exp.Property = propIdent

	// We don't call parseExpression here because the right side MUST be an identifier.
	// The precedence check in the main parseExpression loop handles chaining, e.g., a?.b?.c
	return exp
}

// isForOfLoop looks ahead to determine if this is a for...of loop
func (p *Parser) isForOfLoop() bool {
	// Simple heuristic: look at tokens after 'for ('
	// We'll parse minimally and reset if it's not for...of

	// We're currently at 'for', check if next is '('
	if !p.peekTokenIs(lexer.LPAREN) {
		return false
	}

	// We need to look ahead more carefully
	// For now, let's use a simpler approach: try to parse and handle errors
	return true // We'll detect inside parseForOfStatement and fallback
}

// parseForOfStatement parses for...of loops
func (p *Parser) parseForOfStatement() *ForStatement {
	debugPrint("parseForOfStatement: START, cur='%s'", p.curToken.Literal)

	// Create ForOfStatement
	stmt := &ForOfStatement{Token: p.curToken} // 'for'

	if !p.expectPeek(lexer.LPAREN) { // Consume '(', cur='('
		debugPrint("parseForOfStatement: ERROR expected LPAREN")
		return nil
	}
	debugPrint("parseForOfStatement: Consumed '(', cur='%s', peek='%s'", p.curToken.Literal, p.peekToken.Literal)

	// Parse variable declaration or identifier
	p.nextToken() // Move past '('
	debugPrint("parseForOfStatement: Variable START, cur='%s'", p.curToken.Literal)

	if p.curTokenIs(lexer.LET) {
		// Parse let declaration
		letStmt := &LetStatement{Token: p.curToken}
		if !p.expectPeek(lexer.IDENT) {
			debugPrint("parseForOfStatement: ERROR expected IDENT after let")
			return nil
		}
		letStmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		// Note: No type annotation or value assignment in for...of
		stmt.Variable = letStmt
	} else if p.curTokenIs(lexer.CONST) {
		// Parse const declaration
		constStmt := &ConstStatement{Token: p.curToken}
		if !p.expectPeek(lexer.IDENT) {
			debugPrint("parseForOfStatement: ERROR expected IDENT after const")
			return nil
		}
		constStmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		stmt.Variable = constStmt
	} else if p.curTokenIs(lexer.IDENT) {
		// Parse bare identifier (reusing existing variable)
		ident := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		exprStmt := &ExpressionStatement{Token: p.curToken, Expression: ident}
		stmt.Variable = exprStmt
	} else {
		debugPrint("parseForOfStatement: ERROR expected variable declaration or identifier")
		return nil
	}

	// Expect 'of'
	if !p.expectPeek(lexer.OF) {
		debugPrint("parseForOfStatement: ERROR expected OF")
		return nil
	}
	debugPrint("parseForOfStatement: Found 'of', cur='%s'", p.curToken.Literal)

	// Parse iterable expression
	p.nextToken() // Move past 'of'
	debugPrint("parseForOfStatement: Parsing iterable, cur='%s'", p.curToken.Literal)
	stmt.Iterable = p.parseExpression(LOWEST)

	// Expect ')'
	if !p.expectPeek(lexer.RPAREN) {
		debugPrint("parseForOfStatement: ERROR expected RPAREN")
		return nil
	}
	debugPrint("parseForOfStatement: Found ')', cur='%s'", p.curToken.Literal)

	// Parse body (same logic as regular for loop)
	if p.peekTokenIs(lexer.LBRACE) {
		// Block statement case
		if !p.expectPeek(lexer.LBRACE) {
			debugPrint("parseForOfStatement: ERROR expected LBRACE for body")
			return nil
		}
		stmt.Body = p.parseBlockStatement()
	} else {
		// Single statement case
		p.nextToken() // Move to the start of the statement
		debugPrint("parseForOfStatement: Parsing single body statement, cur='%s'", p.curToken.Literal)
		bodyStmt := p.parseStatement()
		if bodyStmt == nil {
			debugPrint("parseForOfStatement: ERROR parsing single body statement")
			return nil
		}
		// Wrap the single statement in a BlockStatement
		stmt.Body = &BlockStatement{
			Token:               p.curToken,
			Statements:          []Statement{bodyStmt},
			HoistedDeclarations: make(map[string]Expression),
		}
	}

	debugPrint("parseForOfStatement: FINISHED")

	// Return as *ForStatement for now - we'll need to handle this in type system
	return (*ForStatement)(unsafe.Pointer(stmt))
}

// parseForStatementOrForOf determines if this is for...of, for...in, or regular for and parses accordingly
func (p *Parser) parseForStatementOrForOf(forToken lexer.Token) Statement {
	// We're positioned at the variable declaration or identifier
	// Parse the variable part and see what comes next

	var varStmt Statement
	var varName string

	if p.curTokenIs(lexer.LET) {
		letStmt := &LetStatement{Token: p.curToken}
		if !p.expectPeek(lexer.IDENT) {
			return nil
		}
		letStmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		varStmt = letStmt
		varName = p.curToken.Literal
	} else if p.curTokenIs(lexer.CONST) {
		constStmt := &ConstStatement{Token: p.curToken}
		if !p.expectPeek(lexer.IDENT) {
			return nil
		}
		constStmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		varStmt = constStmt
		varName = p.curToken.Literal
	} else if p.curTokenIs(lexer.IDENT) {
		ident := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		exprStmt := &ExpressionStatement{Token: p.curToken, Expression: ident}
		varStmt = exprStmt
		varName = p.curToken.Literal
	} else {
		return nil
	}

	// Check what comes after the variable
	if p.peekTokenIs(lexer.OF) {
		// This is a for...of loop!
		p.nextToken() // consume variable name, cur='of'

		stmt := &ForOfStatement{Token: forToken}
		stmt.Variable = varStmt

		// Parse iterable
		p.nextToken() // consume 'of', move to iterable
		stmt.Iterable = p.parseExpression(LOWEST)

		// Expect ')'
		if !p.expectPeek(lexer.RPAREN) {
			return nil
		}

		// Parse body
		stmt.Body = p.parseForBody()

		// Return ForOfStatement properly
		return stmt
	} else if p.peekTokenIs(lexer.IN) {
		// This is a for...in loop!
		p.nextToken() // consume variable name, cur='in'

		stmt := &ForInStatement{Token: forToken}
		stmt.Variable = varStmt

		// Parse object
		p.nextToken() // consume 'in', move to object
		stmt.Object = p.parseExpression(LOWEST)

		// Expect ')'
		if !p.expectPeek(lexer.RPAREN) {
			return nil
		}

		// Parse body
		stmt.Body = p.parseForBody()

		// Return ForInStatement properly
		return stmt
	} else {
		// This is a regular for loop with variable declaration
		// We need to continue parsing as regular for loop
		// Reset and parse as regular for statement
		return p.parseRegularForStatementWithVar(forToken, varStmt, varName)
	}
}

// parseRegularForStatement parses a standard C-style for loop
func (p *Parser) parseRegularForStatement(forToken lexer.Token) *ForStatement {
	stmt := &ForStatement{Token: forToken}

	// We're at '(' already consumed, now parse the rest
	debugPrint("parseRegularForStatement: START")

	// --- 1. Parse Initializer ---
	if !p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken() // Move to start of initializer
		if p.curTokenIs(lexer.LET) {
			letStmt := &LetStatement{Token: p.curToken}
			if !p.expectPeek(lexer.IDENT) {
				return nil
			}
			letStmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken()
				p.nextToken()
				letStmt.TypeAnnotation = p.parseTypeExpression()
			}
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken()
				p.nextToken()
				letStmt.Value = p.parseExpression(LOWEST)
			}
			stmt.Initializer = letStmt
		} else {
			exprStmt := &ExpressionStatement{Token: p.curToken}
			exprStmt.Expression = p.parseExpression(LOWEST)
			stmt.Initializer = exprStmt
		}
		if !p.expectPeek(lexer.SEMICOLON) {
			return nil
		}
	} else {
		p.nextToken() // consume first ';'
		stmt.Initializer = nil
	}

	// --- 2. Parse Condition ---
	if !p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
		stmt.Condition = p.parseExpression(LOWEST)
		if !p.expectPeek(lexer.SEMICOLON) {
			return nil
		}
	} else {
		p.nextToken() // consume second ';'
		stmt.Condition = nil
	}

	// --- 3. Parse Update ---
	if !p.peekTokenIs(lexer.RPAREN) {
		p.nextToken()
		stmt.Update = p.parseExpression(LOWEST)
		if !p.expectPeek(lexer.RPAREN) {
			return nil
		}
	} else {
		p.nextToken() // consume ')'
		stmt.Update = nil
	}

	// Parse body
	stmt.Body = p.parseForBody()

	return stmt
}

// parseRegularForStatementWithVar parses regular for loop when we already parsed a variable
func (p *Parser) parseRegularForStatementWithVar(forToken lexer.Token, varStmt Statement, varName string) *ForStatement {
	stmt := &ForStatement{Token: forToken}
	stmt.Initializer = varStmt

	// Continue parsing the initializer (might have type annotation or assignment)
	if p.peekTokenIs(lexer.COLON) {
		// Handle type annotation for let statements
		if letStmt, ok := varStmt.(*LetStatement); ok {
			p.nextToken()
			p.nextToken()
			letStmt.TypeAnnotation = p.parseTypeExpression()
		}
	}
	if p.peekTokenIs(lexer.ASSIGN) {
		// Handle assignment
		p.nextToken()
		p.nextToken()
		if letStmt, ok := varStmt.(*LetStatement); ok {
			letStmt.Value = p.parseExpression(LOWEST)
		} else if constStmt, ok := varStmt.(*ConstStatement); ok {
			constStmt.Value = p.parseExpression(LOWEST)
		}
		// For expression statements, we'd need to create an assignment expression
	}

	// Expect semicolon after initializer
	if !p.expectPeek(lexer.SEMICOLON) {
		return nil
	}

	// Continue with condition and update parsing
	if !p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
		stmt.Condition = p.parseExpression(LOWEST)
		if !p.expectPeek(lexer.SEMICOLON) {
			return nil
		}
	} else {
		p.nextToken()
		stmt.Condition = nil
	}

	if !p.peekTokenIs(lexer.RPAREN) {
		p.nextToken()
		stmt.Update = p.parseExpression(LOWEST)
		if !p.expectPeek(lexer.RPAREN) {
			return nil
		}
	} else {
		p.nextToken()
		stmt.Update = nil
	}

	stmt.Body = p.parseForBody()
	return stmt
}

// parseForBody parses the body of any for loop
func (p *Parser) parseForBody() *BlockStatement {
	if p.peekTokenIs(lexer.LBRACE) {
		if !p.expectPeek(lexer.LBRACE) {
			return nil
		}
		return p.parseBlockStatement()
	} else {
		// Single statement
		p.nextToken()
		bodyStmt := p.parseStatement()
		if bodyStmt == nil {
			return nil
		}
		return &BlockStatement{
			Token:               p.curToken,
			Statements:          []Statement{bodyStmt},
			HoistedDeclarations: make(map[string]Expression),
		}
	}
}

// parseMethodTypeSignature parses method type signatures like methodName(param: Type): ReturnType
// This is different from parseFunctionTypeExpression which uses arrow syntax
func (p *Parser) parseMethodTypeSignature() Expression {
	// Current token should be '(' when this is called
	if !p.curTokenIs(lexer.LPAREN) {
		p.addError(p.curToken, "expected '(' for method signature")
		return nil
	}

	// Parse parameter list (similar to parseFunctionTypeParameterList)
	params := []Expression{}

	// Handle empty parameter list: () : ...
	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken() // Consume ')'
	} else {
		// Parse first parameter type
		p.nextToken() // Consume '('

		// Handle optional parameter name
		if p.curTokenIs(lexer.IDENT) && p.peekTokenIs(lexer.COLON) {
			p.nextToken() // Consume IDENT (parameter name, ignored for type)
			p.nextToken() // Consume ':', move to the actual type
		} // Now curToken should be the start of the type expression

		paramType := p.parseTypeExpression()
		if paramType == nil {
			return nil
		}
		params = append(params, paramType)

		// Parse subsequent parameter types
		for p.peekTokenIs(lexer.COMMA) {
			p.nextToken() // Consume ','
			p.nextToken() // Move to next token

			// Handle optional parameter name
			if p.curTokenIs(lexer.IDENT) && p.peekTokenIs(lexer.COLON) {
				p.nextToken() // Consume IDENT
				p.nextToken() // Consume ':', move to the actual type
			}

			paramType := p.parseTypeExpression()
			if paramType == nil {
				return nil
			}
			params = append(params, paramType)
		}

		// Expect closing parenthesis
		if !p.expectPeek(lexer.RPAREN) {
			return nil
		}
	}

	// Now expect ':' for return type (not '=>' like in arrow functions)
	if !p.expectPeek(lexer.COLON) {
		return nil
	}

	// Parse the return type
	p.nextToken() // Move to the start of the return type expression
	returnType := p.parseTypeExpression()
	if returnType == nil {
		return nil
	}

	// Create a FunctionTypeExpression to represent the method signature
	funcType := &FunctionTypeExpression{
		Token:      lexer.Token{Type: lexer.LPAREN, Literal: "("},
		Parameters: params,
		ReturnType: returnType,
	}

	return funcType
}

// GetTokenFromNode attempts to extract the primary token associated with a parser node.
// This is useful for getting line numbers for error reporting.
// Returns the zero value of lexer.Token if no specific token can be easily extracted.
func GetTokenFromNode(node Node) lexer.Token {
	switch n := node.(type) {
	// Statements (use the primary keyword/token)
	case *LetStatement:
		return n.Token
	case *ConstStatement:
		return n.Token
	case *VarStatement:
		return n.Token
	case *ReturnStatement:
		return n.Token
	case *ExpressionStatement:
		if n.Expression != nil {
			return GetTokenFromNode(n.Expression) // Use expression's token recursively
		}
		return n.Token // Fallback to statement token (often start of expression)
	case *BlockStatement:
		return n.Token // The '{' token
	case *IfExpression:
		return n.Token // The 'if' token
	case *WhileStatement:
		return n.Token // 'while' token
	case *ForStatement:
		return n.Token // 'for' token
	case *ForOfStatement:
		return n.Token // 'for' token
	case *BreakStatement:
		return n.Token // 'break' token
	case *ContinueStatement:
		return n.Token // 'continue' token
	case *DoWhileStatement:
		return n.Token // 'do' token
	case *TypeAliasStatement:
		return n.Token // 'type' token
	case *InterfaceDeclaration:
		return n.Token // 'interface' token
	case *SwitchStatement:
		return n.Token // 'switch' token

	// Expressions (use the primary token where available)
	case *Identifier:
		return n.Token
	case *NumberLiteral:
		return n.Token
	case *StringLiteral:
		return n.Token
	case *TemplateLiteral:
		return n.Token
	case *BooleanLiteral:
		return n.Token
	case *NullLiteral:
		return n.Token
	case *UndefinedLiteral:
		return n.Token
	case *ThisExpression:
		return n.Token
	case *ObjectLiteral:
		return n.Token // The '{' token
	case *ShorthandMethod:
		return n.Token // The method name token
	case *FunctionLiteral:
		return n.Token // The 'function' token
	case *FunctionSignature:
		return n.Token // The 'function' token
	case *ArrowFunctionLiteral:
		return n.Token // The '=>' token
	case *PrefixExpression:
		return n.Token // The operator token
	case *TypeofExpression:
		return n.Token // The 'typeof' token
	case *InfixExpression:
		return n.Token // The operator token
	case *TernaryExpression:
		return n.Token // The '?' token
	case *CallExpression:
		return n.Token // The '(' token
	case *NewExpression:
		return n.Token // The 'new' token
	case *IndexExpression:
		return n.Token // The '[' token
	case *ArrayLiteral:
		return n.Token // The '[' token
	case *MemberExpression:
		return n.Token // The '.' token
	case *OptionalChainingExpression:
		return n.Token // The '?.' token
	case *AssignmentExpression:
		return n.Token // The assignment operator token
	case *UpdateExpression:
		return n.Token // The update operator token
	case *SpreadElement:
		return n.Token // The '...' token

	// Type expressions
	case *UnionTypeExpression:
		return n.Token // The '|' token
	case *ArrayTypeExpression:
		return n.Token // The '[' token
	case *FunctionTypeExpression:
		return n.Token // The '(' token
	case *ObjectTypeExpression:
		return n.Token // The '{' token
	case *ConstructorTypeExpression:
		return n.Token // The 'new' token

	// Special cases
	case *Program:
		if len(n.Statements) > 0 {
			return GetTokenFromNode(n.Statements[0]) // Use first statement's token
		}
		return lexer.Token{} // Empty program, return zero value

	// Add other node types as needed
	default:
		// Cannot easily determine a representative token
		return lexer.Token{} // Return zero value
	}
}
