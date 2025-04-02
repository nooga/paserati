package parser

import (
	"fmt"
	"paserati/pkg/errors"
	"paserati/pkg/lexer"
	"strconv"
	"strings"
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

	// Pratt parser needs prefix and infix parse functions maps
	prefixParseFns map[lexer.TokenType]prefixParseFn
	infixParseFns  map[lexer.TokenType]infixParseFn
}

// Parsing functions types for Pratt parser
type (
	prefixParseFn func() Expression
	infixParseFn  func(Expression) Expression // Arg is the left side expression
)

// Precedence levels for operators
const (
	_ int = iota
	LOWEST
	ASSIGNMENT  // =
	TERNARY     // ?:
	COALESCE    // ?? (Added)
	LOGICAL_OR  // || (Added)
	LOGICAL_AND // && (Added)
	EQUALS      // ==, !=, ===, !==
	LESSGREATER // > or < or <= or >=
	SUM         // + or -
	PRODUCT     // * or /
	PREFIX      // -X or !X or ++X or --X
	POSTFIX     // X++ or X-- (Higher than prefix/call)
	CALL        // myFunction(X)
	INDEX       // array[index]
	MEMBER      // object.property
)

// --- NEW: Type Precedence ---
const (
	_ int = iota
	TYPE_LOWEST
	TYPE_UNION // |
	TYPE_ARRAY // [] (Higher precedence than union)
)

// Precedences map for operator tokens
var precedences = map[lexer.TokenType]int{
	lexer.ASSIGN:          ASSIGNMENT,
	lexer.PLUS_ASSIGN:     ASSIGNMENT,
	lexer.MINUS_ASSIGN:    ASSIGNMENT,
	lexer.ASTERISK_ASSIGN: ASSIGNMENT,
	lexer.SLASH_ASSIGN:    ASSIGNMENT,
	lexer.EQ:              EQUALS,
	lexer.NOT_EQ:          EQUALS,
	lexer.STRICT_EQ:       EQUALS,
	lexer.STRICT_NOT_EQ:   EQUALS,
	lexer.LT:              LESSGREATER,
	lexer.GT:              LESSGREATER,
	lexer.LE:              LESSGREATER,
	lexer.GE:              LESSGREATER,
	lexer.PLUS:            SUM,
	lexer.MINUS:           SUM,
	lexer.SLASH:           PRODUCT,
	lexer.ASTERISK:        PRODUCT,
	lexer.LPAREN:          CALL,
	lexer.LBRACKET:        INDEX,
	lexer.DOT:             MEMBER,
	lexer.QUESTION:        TERNARY,
	lexer.LOGICAL_AND:     LOGICAL_AND,
	lexer.LOGICAL_OR:      LOGICAL_OR,
	lexer.COALESCE:        COALESCE,
	lexer.INC:             POSTFIX,
	lexer.DEC:             POSTFIX,
}

// NewParser creates a new Parser.
func NewParser(l *lexer.Lexer) *Parser {
	p := &Parser{
		l:      l,
		errors: []errors.PaseratiError{},
	}

	// Initialize Pratt parser maps
	p.prefixParseFns = make(map[lexer.TokenType]prefixParseFn)
	p.registerPrefix(lexer.IDENT, p.parseIdentifier)
	p.registerPrefix(lexer.NUMBER, p.parseNumberLiteral)
	p.registerPrefix(lexer.STRING, p.parseStringLiteral)
	p.registerPrefix(lexer.TRUE, p.parseBooleanLiteral)
	p.registerPrefix(lexer.FALSE, p.parseBooleanLiteral)
	p.registerPrefix(lexer.NULL, p.parseNullLiteral)
	p.registerPrefix(lexer.UNDEFINED, p.parseUndefinedLiteral)
	p.registerPrefix(lexer.FUNCTION, p.parseFunctionLiteral)
	p.registerPrefix(lexer.BANG, p.parsePrefixExpression)      // !true
	p.registerPrefix(lexer.MINUS, p.parsePrefixExpression)     // -5
	p.registerPrefix(lexer.INC, p.parsePrefixUpdateExpression) // Added ++x
	p.registerPrefix(lexer.DEC, p.parsePrefixUpdateExpression) // Added --x
	p.registerPrefix(lexer.LPAREN, p.parseGroupedExpression)   // (5 + 5)
	p.registerPrefix(lexer.IF, p.parseIfExpression)            // if (condition) { ... }
	p.registerPrefix(lexer.LBRACKET, p.parseArrayLiteral)      // Added [1, 2]

	p.infixParseFns = make(map[lexer.TokenType]infixParseFn)
	p.registerInfix(lexer.PLUS, p.parseInfixExpression)
	p.registerInfix(lexer.MINUS, p.parseInfixExpression)
	p.registerInfix(lexer.SLASH, p.parseInfixExpression)
	p.registerInfix(lexer.ASTERISK, p.parseInfixExpression)
	p.registerInfix(lexer.EQ, p.parseInfixExpression)
	p.registerInfix(lexer.NOT_EQ, p.parseInfixExpression)
	p.registerInfix(lexer.STRICT_EQ, p.parseInfixExpression)
	p.registerInfix(lexer.STRICT_NOT_EQ, p.parseInfixExpression)
	p.registerInfix(lexer.LT, p.parseInfixExpression)
	p.registerInfix(lexer.GT, p.parseInfixExpression)
	p.registerInfix(lexer.LE, p.parseInfixExpression)
	p.registerInfix(lexer.GE, p.parseInfixExpression)
	p.registerInfix(lexer.LPAREN, p.parseCallExpression)
	p.registerInfix(lexer.QUESTION, p.parseTernaryExpression)
	p.registerInfix(lexer.ASSIGN, p.parseAssignmentExpression)
	p.registerInfix(lexer.PLUS_ASSIGN, p.parseAssignmentExpression)
	p.registerInfix(lexer.MINUS_ASSIGN, p.parseAssignmentExpression)
	p.registerInfix(lexer.ASTERISK_ASSIGN, p.parseAssignmentExpression)
	p.registerInfix(lexer.SLASH_ASSIGN, p.parseAssignmentExpression)
	p.registerInfix(lexer.LOGICAL_AND, p.parseInfixExpression)
	p.registerInfix(lexer.LOGICAL_OR, p.parseInfixExpression)
	p.registerInfix(lexer.COALESCE, p.parseInfixExpression)
	p.registerInfix(lexer.INC, p.parsePostfixUpdateExpression) // Added x++
	p.registerInfix(lexer.DEC, p.parsePostfixUpdateExpression) // Added x--
	p.registerInfix(lexer.LBRACKET, p.parseIndexExpression)    // Added arr[idx]
	p.registerInfix(lexer.DOT, p.parseMemberExpression)        // <<< Added registration

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

	for p.curToken.Type != lexer.EOF {
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
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
	case lexer.RETURN:
		return p.parseReturnStatement()
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
	default:
		return p.parseExpressionStatement()
	}
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

// --- NEW: Type Expression Parsing (Placeholder) ---

// parseTypeExpression parses a type annotation, potentially including union types.
func (p *Parser) parseTypeExpression() Expression {
	return p.parseTypeExpressionRecursive(TYPE_LOWEST)
}

// parseTypeExpressionRecursive handles precedence for type operators.
func (p *Parser) parseTypeExpressionRecursive(precedence int) Expression {
	debugPrint("parseTypeExpressionRecursive(prec=%d): START, cur='%s'", precedence, p.curToken.Literal)

	// Parse the initial type (prefix part)
	var leftExp Expression
	switch p.curToken.Type {
	case lexer.IDENT, lexer.NULL, lexer.UNDEFINED:
		leftExp = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	// --- NEW: Handle Literal Types ---
	case lexer.STRING:
		leftExp = p.parseStringLiteral()
	case lexer.NUMBER:
		leftExp = p.parseNumberLiteral()
	case lexer.TRUE, lexer.FALSE:
		leftExp = p.parseBooleanLiteral()
	// --- End Literal Types ---
	// TODO: Add cases for parsing array types `T[]`, function types `(...) => T`, object types `{...}` here.
	// case lexer.LBRACKET: leftExp = p.parseArrayTypeExpression()
	default:
		// Error: Expected a type identifier, array, function, or object type start
		msg := fmt.Sprintf("unexpected token %s (%q) at start of type annotation",
			p.curToken.Type, p.curToken.Literal)
		p.addError(p.curToken, msg)
		debugPrint("parseTypeExpressionRecursive: ERROR - %s", msg)
		return nil
	}
	debugPrint("parseTypeExpressionRecursive: Parsed prefix type %T ('%s')", leftExp, leftExp.String())

	// Look for infix type operators (like '|')
	for precedence < p.peekTypePrecedence() {
		peekType := p.peekToken.Type
		debugPrint("parseTypeExpressionRecursive: Found infix type operator '%s'", peekType)

		switch peekType {
		case lexer.PIPE:
			p.nextToken() // Consume '|'
			leftExp = p.parseUnionTypeExpression(leftExp)
			if leftExp == nil {
				return nil // Error during union parsing
			}
		case lexer.LBRACKET: // Handle T[] syntax
			p.nextToken()                                 // Consume '['
			leftExp = p.parseArrayTypeExpression(leftExp) // Need to implement this
			if leftExp == nil {
				return nil // Error during array type parsing
			}
		// TODO: Add cases for other potential infix type operators (e.g., '&' for intersection)
		default:
			// No infix operator found or lower precedence
			return leftExp
		}
		debugPrint("parseTypeExpressionRecursive: After infix, leftExp=%T ('%s')", leftExp, leftExp.String())
	}

	return leftExp
}

// --- NEW: Helper for infix union type parsing ---
func (p *Parser) parseUnionTypeExpression(left Expression) Expression {
	unionExp := &UnionTypeExpression{
		Token: p.curToken, // The '|' token
		Left:  left,
	}
	precedence := TYPE_UNION
	p.nextToken() // Consume the token starting the right-hand side type
	unionExp.Right = p.parseTypeExpressionRecursive(precedence)
	if unionExp.Right == nil {
		return nil // Error parsing right side
	}
	return unionExp
}

// --- NEW: Precedence helper for type operators ---
func (p *Parser) peekTypePrecedence() int {
	switch p.peekToken.Type {
	case lexer.PIPE:
		return TYPE_UNION
	case lexer.LBRACKET: // For T[] syntax
		return TYPE_ARRAY
	// TODO: Add other type operators (e.g., lexer.AMPERSAND for TYPE_INTERSECTION)
	default:
		return TYPE_LOWEST
	}
}

// --- NEW: Helper for infix array type parsing T[] ---
func (p *Parser) parseArrayTypeExpression(elementType Expression) Expression {
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
	debugPrint("parseIdentifier: cur='%s', peek='%s' (%s)", p.curToken.Literal, p.peekToken.Literal, p.peekToken.Type)

	if p.peekTokenIs(lexer.ARROW) {
		debugPrint("parseIdentifier: Found '=>' after identifier '%s'", ident.Value)
		// This handles the case like: x => ...
		// Here 'x' is the single parameter.
		p.nextToken() // Consume the identifier token (which is curToken)
		debugPrint("parseIdentifier: Consumed IDENT, cur is now '%s' (%s)", p.curToken.Literal, p.curToken.Type)

		// --- FIX: Create Parameter node for shorthand arrow function ---
		param := &Parameter{
			Token:          ident.Token,
			Name:           ident,
			TypeAnnotation: nil, // No type annotation in this shorthand syntax
		}
		return p.parseArrowFunctionBodyAndFinish([]*Parameter{param}, nil)
	}

	debugPrint("parseIdentifier: Just identifier '%s', returning.", ident.Value)
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

func (p *Parser) parseBooleanLiteral() Expression {
	return &BooleanLiteral{Token: p.curToken, Value: p.curTokenIs(lexer.TRUE)}
}

func (p *Parser) parseNullLiteral() Expression {
	return &NullLiteral{Token: p.curToken}
}

func (p *Parser) parseUndefinedLiteral() Expression {
	return &UndefinedLiteral{Token: p.curToken}
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
	lit.Parameters = p.parseFunctionParameters() // Includes consuming RPAREN
	if lit.Parameters == nil {
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

	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	lit.Body = p.parseBlockStatement() // Includes consuming RBRACE

	return lit
}

// --- MODIFIED: parseFunctionParameters to handle Parameter struct & types ---
func (p *Parser) parseFunctionParameters() []*Parameter {
	parameters := []*Parameter{}

	// Check for empty parameter list: function() { ... }
	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken() // Consume ')'
		return parameters
	}

	p.nextToken() // Consume '(' or ',' to get to the first parameter name

	// Parse first parameter
	if !p.curTokenIs(lexer.IDENT) {
		msg := fmt.Sprintf("expected identifier for parameter name, got %s", p.curToken.Type)
		p.addError(p.curToken, msg)
		debugPrint("parseParameterList: Error - %s", msg)
		return nil
	}
	param := &Parameter{Token: p.curToken}
	param.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check for Type Annotation
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ':'
		p.nextToken() // Consume token starting the type expression
		param.TypeAnnotation = p.parseTypeExpression()
		if param.TypeAnnotation == nil {
			return nil
		} // Propagate error
	} else {
		param.TypeAnnotation = nil
	}
	parameters = append(parameters, param)

	// Subsequent parameters
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken() // Consume ','
		p.nextToken() // Consume identifier for next param name

		if !p.curTokenIs(lexer.IDENT) {
			msg := fmt.Sprintf("expected identifier for parameter name after comma, got %s", p.curToken.Type)
			p.addError(p.curToken, msg)
			debugPrint("parseParameterList: Error - %s", msg)
			return nil
		}
		param := &Parameter{Token: p.curToken}
		param.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

		// Check for Type Annotation
		if p.peekTokenIs(lexer.COLON) {
			p.nextToken() // Consume ':'
			p.nextToken() // Consume token starting the type expression
			param.TypeAnnotation = p.parseTypeExpression()
			if param.TypeAnnotation == nil {
				return nil
			} // Propagate error
		} else {
			param.TypeAnnotation = nil
		}
		parameters = append(parameters, param)
	}

	if !p.expectPeek(lexer.RPAREN) {
		return nil // Expected closing parenthesis
	}

	return parameters
}

func (p *Parser) parseBlockStatement() *BlockStatement {
	block := &BlockStatement{Token: p.curToken} // The '{' token
	block.Statements = []Statement{}

	p.nextToken() // Consume '{'

	for !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
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
		params := p.parseParameterList() // Consumes up to and including ')'

		if params != nil && p.curTokenIs(lexer.RPAREN) && p.peekTokenIs(lexer.ARROW) {
			debugPrint("parseGroupedExpression: Successfully parsed arrow params: %v, found '=>' next.", params)
			p.nextToken() // Consume '=>', curToken is now '=>'
			debugPrint("parseGroupedExpression: Consumed '=>', cur='%s' (%s)", p.curToken.Literal, p.curToken.Type)
			p.errors = p.errors[:startErrors]
			return p.parseArrowFunctionBodyAndFinish(params, nil)
		} else if params != nil && p.curTokenIs(lexer.RPAREN) && p.peekTokenIs(lexer.COLON) {
			debugPrint("parseGroupedExpression: Successfully parsed arrow params: %v, found ':' next.", params)
			p.nextToken() // Consume ':', curToken is now ':'
			p.nextToken() // Consume the token starting the type expression
			debugPrint("parseGroupedExpression: Consumed ':', cur='%s' (%s)", p.curToken.Literal, p.curToken.Type)
			p.errors = p.errors[:startErrors]

			returnTypeAnnotation := p.parseTypeExpression()

			if returnTypeAnnotation == nil {
				return nil
			} // Propagate error

			p.nextToken() // Consume the token ending the type expression
			debugPrint("parseGroupedExpression: Consumed type expression, cur='%s' (%s)", p.curToken.Literal, p.curToken.Type)

			return p.parseArrowFunctionBodyAndFinish(params, returnTypeAnnotation)
		} else {
			debugPrint("parseGroupedExpression: Failed arrow param parse (params=%v, cur='%s', peek='%s') or no '=>', backtracking...", params, p.curToken.Literal, p.peekToken.Type)
			// --- PRECISE BACKTRACK ---
			p.l.SetPosition(startPos) // Reset lexer position
			p.curToken = startCur     // Restore original curToken
			p.peekToken = startPeek   // Restore original peekToken
			// DO NOT call nextToken() here, state should be exactly as before the attempt.
			// The subsequent code expects curToken to be the starting '('.
			p.errors = p.errors[:startErrors]
			debugPrint("parseGroupedExpression: Precise Backtrack complete. cur='%s', peek='%s'", p.curToken.Literal, p.peekToken.Literal)
		}
	} else {
		debugPrint("parseGroupedExpression: Not starting with '(', cannot be parenthesized arrow params.")
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
		return nil
	} // Propagate error from inner expression
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

	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	debugPrint("parseIfExpression parsing consequence block...")
	expr.Consequence = p.parseBlockStatement()
	if expr.Consequence == nil {
		return nil
	} // <<< NIL CHECK
	debugPrint("parseIfExpression parsed consequence block.")

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
			expr.Alternative = elseBlock
			debugPrint("parseIfExpression parsed 'else if' branch.")

		} else if p.expectPeek(lexer.LBRACE) { // Standard 'else { ... }'
			debugPrint("parseIfExpression parsing standard 'else' block...")
			// Call parseBlockStatement first before assigning
			alternativeBlock := p.parseBlockStatement()

			// --- DEBUG: Log state of block BEFORE assignment ---
			fmt.Printf("// [Parser IfExpr] Assigning Alternative: BlockPtr=%p", alternativeBlock)
			if alternativeBlock != nil {
				statementsPtr := &alternativeBlock.Statements // Get pointer to the slice header
				fmt.Printf(", StmtSliceHeaderPtr=%p", statementsPtr)
				if alternativeBlock.Statements == nil {
					fmt.Printf(", Statements=nil\n")
				} else {
					fmt.Printf(", Statements.Len=%d\n", len(alternativeBlock.Statements))
				}
			} else {
				fmt.Printf(", Block=nil\n")
			}
			// --- END DEBUG ---

			expr.Alternative = alternativeBlock // Assign the parsed block

			if expr.Alternative == nil {
				return nil
			} // <<< NIL CHECK
			debugPrint("parseIfExpression parsed standard 'else' block.")
		} else {
			// Error: expected '{' or 'if' after 'else'
			msg := fmt.Sprintf("expected { or if after else, got %s instead", p.peekToken.Type)
			p.addError(p.peekToken, msg)
			debugPrint("parseIfExpression failed: %s", msg)
			return nil
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
	expression := &InfixExpression{
		Token:    p.curToken, // The operator token
		Operator: p.curToken.Literal,
		Left:     left,
	}

	precedence := p.curPrecedence()
	p.nextToken()                                    // Consume the operator
	expression.Right = p.parseExpression(precedence) // Parse the right operand

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
func (p *Parser) parseArrowFunctionBodyAndFinish(params []*Parameter, returnTypeAnnotation Expression) Expression {
	debugPrint("parseArrowFunctionBodyAndFinish: Starting, curToken='%s' (%s), params=%v", p.curToken.Literal, p.curToken.Type, params)
	arrowFunc := &ArrowFunctionLiteral{
		Token:                p.curToken, // The '=>' token
		Parameters:           params,     // Use the passed-in parameters
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
// Returns the list of identifier nodes or nil if parsing fails.
func (p *Parser) parseParameterList() []*Parameter {
	params := []*Parameter{}

	if !p.curTokenIs(lexer.LPAREN) { // Check current token IS LPAREN
		// This case should ideally not be hit if called correctly from parseGroupedExpression
		return nil
	}
	debugPrint("parseParameterList: Starting, cur='%s', peek='%s'", p.curToken.Literal, p.peekToken.Literal)

	// Handle empty list: () => ...
	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken() // Consume ')'
		debugPrint("parseParameterList: Found empty list '()'")
		return params // Return empty slice
	}

	// Parse the first parameter
	p.nextToken() // Move past '(' to the first parameter identifier
	if !p.curTokenIs(lexer.IDENT) {
		msg := fmt.Sprintf("expected identifier as parameter, got %s", p.curToken.Type)
		p.addError(p.curToken, msg)
		debugPrint("parseParameterList: Error - %s", msg)
		return nil
	}
	param := &Parameter{Token: p.curToken}
	param.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check for Type Annotation
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ':'
		p.nextToken() // Consume token starting the type expression
		param.TypeAnnotation = p.parseTypeExpression()
		if param.TypeAnnotation == nil {
			return nil
		} // Propagate error
	} else {
		param.TypeAnnotation = nil
	}
	params = append(params, param)
	debugPrint("parseParameterList: Parsed param '%s' (type: %v)", param.Name.Value, param.TypeAnnotation)

	// Parse subsequent parameters (comma-separated)
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken() // Consume ','
		p.nextToken() // Consume identifier
		if !p.curTokenIs(lexer.IDENT) {
			msg := fmt.Sprintf("expected identifier for parameter name after comma, got %s", p.curToken.Type)
			p.addError(p.curToken, msg)
			debugPrint("parseParameterList: Error - %s", msg)
			return nil
		}
		param := &Parameter{Token: p.curToken}
		param.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

		// Check for Type Annotation
		if p.peekTokenIs(lexer.COLON) {
			p.nextToken() // Consume ':'
			p.nextToken() // Consume token starting the type expression
			param.TypeAnnotation = p.parseTypeExpression()
			if param.TypeAnnotation == nil {
				return nil
			} // Propagate error
		} else {
			param.TypeAnnotation = nil
		}
		params = append(params, param)
		debugPrint("parseParameterList: Parsed param '%s' (type: %v)", param.Name.Value, param.TypeAnnotation)
	}

	// Expect closing parenthesis
	if !p.expectPeek(lexer.RPAREN) {
		debugPrint("parseParameterList: Expected ')' after parameters, got peek '%s'", p.peekToken.Type)
		return nil // Error: Missing closing parenthesis
	}
	debugPrint("parseParameterList: Consumed ')', finished successfully.")

	return params
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

	// Check if the left side is assignable (Identifier or IndexExpression)
	validLHS := false
	switch left.(type) {
	case *Identifier:
		validLHS = true
	case *IndexExpression:
		// Allow simple assignment (=) for index expressions
		if expr.Operator == "=" {
			validLHS = true
		} else {
			msg := fmt.Sprintf("operator %s cannot be applied to index expression", expr.Operator)
			p.addError(expr.Token, msg)
			return nil
		}
		// TODO: Add case for MemberExpression later
	case *MemberExpression:
		validLHS = true
	}

	if !validLHS {
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

	if !p.expectPeek(lexer.LBRACE) {
		return nil // Expected '{' to start the body
	}

	stmt.Body = p.parseBlockStatement() // parseBlockStatement handles '{' ... '}'

	return stmt
}

// --- New: For Statement Parsing ---

func (p *Parser) parseForStatement() *ForStatement {
	debugPrint("parseForStatement: START, cur='%s'", p.curToken.Literal)
	stmt := &ForStatement{Token: p.curToken} // 'for'

	if !p.expectPeek(lexer.LPAREN) { // Consume '(', cur='('
		debugPrint("parseForStatement: ERROR expected LPAREN")
		return nil
	}
	debugPrint("parseForStatement: Consumed '(', cur='%s', peek='%s'", p.curToken.Literal, p.peekToken.Literal) // Expect cur='('

	// --- 1. Parse Initializer ---
	if !p.peekTokenIs(lexer.SEMICOLON) { // Is there something before the first ';'?
		p.nextToken() // Consume '(', cur=start of initializer
		debugPrint("parseForStatement: Initializer START, cur='%s'", p.curToken.Literal)
		if p.curTokenIs(lexer.LET) {
			// Parse let manually - Copied logic from previous attempt
			letStmt := &LetStatement{Token: p.curToken}
			if !p.expectPeek(lexer.IDENT) {
				debugPrint("parseForStatement: ERROR expected IDENT after let")
				return nil
			} // Consume 'let', cur=IDENT
			letStmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken()
				p.nextToken() // Consume IDENT, ':', cur=Type
				letStmt.TypeAnnotation = p.parseTypeExpression()
			}
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken()
				p.nextToken() // Consume Name/Type, '=', cur=start of value
				letStmt.Value = p.parseExpression(LOWEST)
			} else {
				letStmt.Value = nil
			}
			stmt.Initializer = letStmt
			debugPrint("parseForStatement: Parsed LET Initializer, cur='%s'", p.curToken.Literal)
		} else { // Expression initializer
			exprStmt := &ExpressionStatement{Token: p.curToken}
			exprStmt.Expression = p.parseExpression(LOWEST)
			stmt.Initializer = exprStmt
			debugPrint("parseForStatement: Parsed EXPR Initializer, cur='%s'", p.curToken.Literal)
		}
		// After parsing non-empty init, cur should be last token of init. Expect ';' next.
		if !p.expectPeek(lexer.SEMICOLON) { // Consume ';', cur=';'
			debugPrint("parseForStatement: ERROR expected SEMICOLON after initializer")
			return nil
		}
	} else { // Initializer IS empty
		p.nextToken() // Consume the first ';', cur becomes ';'
		stmt.Initializer = nil
		debugPrint("parseForStatement: Initializer is EMPTY, cur=';'")
	}
	// STATE: cur = first ';'

	// --- 2. Parse Condition ---
	debugPrint("parseForStatement: Condition START, cur='%s', peek='%s'", p.curToken.Literal, p.peekToken.Literal) // Expect cur=';'
	if !p.peekTokenIs(lexer.SEMICOLON) {                                                                           // Is there something between first and second ';'?
		p.nextToken() // Consume first ';', cur=start of condition
		debugPrint("parseForStatement: Parsing Condition, cur='%s'", p.curToken.Literal)
		stmt.Condition = p.parseExpression(LOWEST)
		debugPrint("parseForStatement: Parsed Condition, cur='%s'", p.curToken.Literal)
		// After parsing non-empty condition, cur should be last token of condition. Expect ';' next.
		if !p.expectPeek(lexer.SEMICOLON) { // Consume ';', cur=';'
			debugPrint("parseForStatement: ERROR expected SEMICOLON after condition")
			return nil
		}
	} else { // Condition IS empty
		p.nextToken() // Consume second ';', cur becomes second ';'
		stmt.Condition = nil
		debugPrint("parseForStatement: Condition is EMPTY, cur=';'")
	}
	// STATE: cur = second ';'

	// --- 3. Parse Update ---
	debugPrint("parseForStatement: Update START, cur='%s', peek='%s'", p.curToken.Literal, p.peekToken.Literal) // Expect cur=';'
	if !p.peekTokenIs(lexer.RPAREN) {                                                                           // Is there something between second ';' and ')'?
		p.nextToken() // Consume second ';', cur=start of update
		debugPrint("parseForStatement: Parsing Update, cur='%s'", p.curToken.Literal)
		stmt.Update = p.parseExpression(LOWEST)
		debugPrint("parseForStatement: Parsed Update, cur='%s'", p.curToken.Literal)
		// After parsing non-empty update, cur should be last token of update. Expect ')' next.
		if !p.expectPeek(lexer.RPAREN) { // Consume ')', cur=')'
			debugPrint("parseForStatement: ERROR expected RPAREN after update")
			return nil
		}
	} else { // Update IS empty
		p.nextToken() // Consume ')', cur becomes ')'
		stmt.Update = nil
		debugPrint("parseForStatement: Update is EMPTY, cur=')'")
	}
	// STATE: cur = ')'

	// --- 4. Parse Body ---
	debugPrint("parseForStatement: Body START, cur='%s', peek='%s'", p.curToken.Literal, p.peekToken.Literal) // Expect cur=')'
	if !p.expectPeek(lexer.LBRACE) {                                                                          // Consume ')', check/consume '{', cur='{'
		debugPrint("parseForStatement: ERROR expected LBRACE for body")
		return nil
	}
	debugPrint("parseForStatement: Consumed LBRACE, cur='%s'", p.curToken.Literal)

	stmt.Body = p.parseBlockStatement()
	debugPrint("parseForStatement: Parsed Body, FINISHED")

	return stmt
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

	// Expect { after 'do'
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()

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

func (p *Parser) parsePrefixUpdateExpression() Expression {
	expr := &UpdateExpression{
		Token:    p.curToken, // ++ or --
		Operator: p.curToken.Literal,
		Prefix:   true,
	}
	p.nextToken()                             // Consume ++ or --
	expr.Argument = p.parseExpression(PREFIX) // Parse argument with PREFIX precedence

	// Check if argument is assignable (currently just Identifier)
	if _, ok := expr.Argument.(*Identifier); !ok {
		msg := fmt.Sprintf("invalid argument for prefix %s: expected identifier, got %T",
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

	// Check if argument is assignable (currently just Identifier)
	if _, ok := expr.Argument.(*Identifier); !ok {
		msg := fmt.Sprintf("invalid argument for postfix %s: expected identifier, got %T",
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

	if !p.expectPeek(lexer.IDENT) {
		// If the token after '.' is not an identifier, it's a syntax error.
		msg := fmt.Sprintf("expected identifier after '.', got %s", p.peekToken.Type)
		p.addError(p.peekToken, msg)
		return nil
	}

	// Construct the Identifier node for the property
	propIdent := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
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
