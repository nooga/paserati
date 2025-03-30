package parser

import (
	"fmt"
	"paseratti2/pkg/lexer"
	"strconv"
)

// Parser takes a lexer and builds an AST.
type Parser struct {
	l      *lexer.Lexer
	errors []string

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
	EQUALS      // == or !=
	LESSGREATER // > or <
	SUM         // + or -
	PRODUCT     // * or /
	PREFIX      // -X or !X
	CALL        // myFunction(X)
	INDEX       // array[index]
)

// Precedences map for operator tokens
var precedences = map[lexer.TokenType]int{
	lexer.EQ:       EQUALS,
	lexer.NOT_EQ:   EQUALS,
	lexer.LT:       LESSGREATER,
	lexer.GT:       LESSGREATER,
	lexer.LE:       LESSGREATER,
	lexer.PLUS:     SUM,
	lexer.MINUS:    SUM,
	lexer.SLASH:    PRODUCT,
	lexer.ASTERISK: PRODUCT,
	lexer.LPAREN:   CALL, // Added for call expressions
	// lexer.LBRACKET: INDEX, // For index expressions
}

// NewParser creates a new Parser.
func NewParser(l *lexer.Lexer) *Parser {
	p := &Parser{
		l:      l,
		errors: []string{},
	}

	// Initialize Pratt parser maps
	p.prefixParseFns = make(map[lexer.TokenType]prefixParseFn)
	p.registerPrefix(lexer.IDENT, p.parseIdentifier)
	p.registerPrefix(lexer.NUMBER, p.parseNumberLiteral)
	p.registerPrefix(lexer.STRING, p.parseStringLiteral)
	p.registerPrefix(lexer.TRUE, p.parseBooleanLiteral)
	p.registerPrefix(lexer.FALSE, p.parseBooleanLiteral)
	p.registerPrefix(lexer.NULL, p.parseNullLiteral)
	p.registerPrefix(lexer.FUNCTION, p.parseFunctionLiteral)
	p.registerPrefix(lexer.BANG, p.parsePrefixExpression)    // !true
	p.registerPrefix(lexer.MINUS, p.parsePrefixExpression)   // -5
	p.registerPrefix(lexer.LPAREN, p.parseGroupedExpression) // (5 + 5)
	p.registerPrefix(lexer.IF, p.parseIfExpression)          // if (condition) { ... }

	p.infixParseFns = make(map[lexer.TokenType]infixParseFn)
	p.registerInfix(lexer.PLUS, p.parseInfixExpression)
	p.registerInfix(lexer.MINUS, p.parseInfixExpression)
	p.registerInfix(lexer.SLASH, p.parseInfixExpression)
	p.registerInfix(lexer.ASTERISK, p.parseInfixExpression)
	p.registerInfix(lexer.EQ, p.parseInfixExpression)
	p.registerInfix(lexer.NOT_EQ, p.parseInfixExpression)
	p.registerInfix(lexer.LT, p.parseInfixExpression)
	p.registerInfix(lexer.GT, p.parseInfixExpression)
	p.registerInfix(lexer.LE, p.parseInfixExpression)
	p.registerInfix(lexer.LPAREN, p.parseCallExpression) // Added: myFunc( ... )
	// Add index expressions later: LBRACKET

	// Read two tokens, so curToken and peekToken are both set
	p.nextToken()
	p.nextToken()

	return p
}

// Errors returns the list of parsing errors.
func (p *Parser) Errors() []string {
	return p.errors
}

// nextToken advances the current and peek tokens.
func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

// ParseProgram parses the entire input and returns the root Program node.
func (p *Parser) ParseProgram() *Program {
	program := &Program{}
	program.Statements = []Statement{}

	for p.curToken.Type != lexer.EOF {
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		p.nextToken()
	}

	return program
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
	default:
		return p.parseExpressionStatement()
	}
}

func (p *Parser) parseLetStatement() *LetStatement {
	stmt := &LetStatement{Token: p.curToken}

	if !p.expectPeek(lexer.IDENT) {
		return nil
	}

	stmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Optional Type Annotation (simplistic for now: just expect an identifier if colon exists)
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ':'
		p.nextToken() // Consume the type identifier token
		// TODO: Proper type expression parsing
		stmt.TypeAnnotation = p.parseIdentifier() // Assume simple identifier type for now
	}

	// Allow omitting = value, defaulting to undefined
	if p.peekTokenIs(lexer.ASSIGN) {
		p.nextToken() // Consume '='
		p.nextToken() // Consume token starting the expression
		stmt.Value = p.parseExpression(LOWEST)
	} else {
		stmt.Value = nil // No initializer provided, implies undefined
	}

	// Optional semicolon
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseConstStatement() *ConstStatement {
	// Structure is identical to let for now
	stmt := &ConstStatement{Token: p.curToken}

	if !p.expectPeek(lexer.IDENT) {
		return nil
	}

	stmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Optional Type Annotation
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ':'
		p.nextToken() // Consume the type identifier token
		// TODO: Proper type expression parsing
		stmt.TypeAnnotation = p.parseIdentifier() // Assume simple identifier type for now
	}

	if !p.expectPeek(lexer.ASSIGN) {
		return nil
	}

	p.nextToken() // Consume '='

	// TODO: Parse the expression properly
	stmt.Value = p.parseExpression(LOWEST)

	// Optional semicolon
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseReturnStatement() *ReturnStatement {
	stmt := &ReturnStatement{Token: p.curToken}
	p.nextToken() // Consume 'return'

	// Check if the next token indicates the end of the statement (or potentially start of next)
	// Allow `return;` or `return}`
	if p.curTokenIs(lexer.SEMICOLON) || p.curTokenIs(lexer.RBRACE) || p.curTokenIs(lexer.EOF) {
		stmt.ReturnValue = nil // Represents returning undefined implicitly
		// Do not consume semicolon here, let the main loop handle it or expectPeek in block
	} else {
		// Parse the expression if present
		stmt.ReturnValue = p.parseExpression(LOWEST)
		// Optional semicolon after expression
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken()
		}
	}

	return stmt
}

func (p *Parser) parseExpressionStatement() *ExpressionStatement {
	stmt := &ExpressionStatement{Token: p.curToken}

	stmt.Expression = p.parseExpression(LOWEST)

	// Optional semicolon for expression statements
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

// --- Expression Parsing (Pratt Parser) ---

func (p *Parser) parseExpression(precedence int) Expression {
	// Prefix part
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}
	leftExp := prefix()

	// Infix part (the core Pratt loop)
	for !p.peekTokenIs(lexer.SEMICOLON) && precedence < p.peekPrecedence() {
		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			return leftExp // No infix operator found or lower precedence
		}

		p.nextToken() // Consume the infix operator token

		leftExp = infix(leftExp) // Call the infix parse function
	}

	return leftExp
}

// -- Prefix Parse Functions --

func (p *Parser) parseIdentifier() Expression {
	return &Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseNumberLiteral() Expression {
	lit := &NumberLiteral{Token: p.curToken}

	value, err := strconv.ParseFloat(p.curToken.Literal, 64)
	if err != nil {
		msg := fmt.Sprintf("could not parse %q as float64", p.curToken.Literal)
		p.errors = append(p.errors, msg)
		return nil
	}
	lit.Value = value
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

func (p *Parser) parseFunctionLiteral() Expression {
	lit := &FunctionLiteral{Token: p.curToken}

	// Optional Function Name
	if p.peekTokenIs(lexer.IDENT) {
		p.nextToken() // Consume name identifier
		lit.Name = p.parseIdentifier().(*Identifier)
	}

	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	lit.Parameters = p.parseFunctionParameters() // Includes consuming RPAREN

	// Optional Return Type
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ':'
		p.nextToken() // Consume the type identifier token
		// TODO: Proper type expression parsing
		lit.ReturnType = p.parseIdentifier() // Assume simple identifier type for now
	}

	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	lit.Body = p.parseBlockStatement() // Includes consuming RBRACE

	return lit
}

func (p *Parser) parseFunctionParameters() []*Identifier {
	identifiers := []*Identifier{}

	// Check for empty parameter list: function() { ... }
	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken() // Consume ')'
		return identifiers
	}

	p.nextToken() // Consume '(' or ',' to get to the first parameter

	// First parameter
	ident := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	// TODO: Add parameter type parsing here (after colon)
	identifiers = append(identifiers, ident)

	// Subsequent parameters
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken() // Consume ','
		p.nextToken() // Consume identifier
		ident := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		// TODO: Add parameter type parsing here
		identifiers = append(identifiers, ident)
	}

	if !p.expectPeek(lexer.RPAREN) {
		return nil // Expected closing parenthesis
	}

	return identifiers
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
	msg := fmt.Sprintf("line %d: expected next token to be %s, got %s instead",
		p.peekToken.Line, t, p.peekToken.Type)
	p.errors = append(p.errors, msg)
}

func (p *Parser) noPrefixParseFnError(t lexer.TokenType) {
	msg := fmt.Sprintf("line %d: no prefix parse function for %s found", p.curToken.Line, t)
	p.errors = append(p.errors, msg)
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

// parseGroupedExpression handles expressions like (expr)
func (p *Parser) parseGroupedExpression() Expression {
	p.nextToken() // Consume '('

	exp := p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.RPAREN) {
		return nil // Expected closing parenthesis
	}

	return exp
}

// parseIfExpression handles 'if (condition) { consequence } else { alternative }' syntax
func (p *Parser) parseIfExpression() Expression {
	expression := &IfExpression{Token: p.curToken}

	if !p.expectPeek(lexer.LPAREN) { // Expect '(' after 'if'
		return nil
	}

	p.nextToken() // Consume '('
	expression.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.RPAREN) { // Expect ')' after condition
		return nil
	}

	if !p.expectPeek(lexer.LBRACE) { // Expect '{' for consequence block
		return nil
	}

	expression.Consequence = p.parseBlockStatement()

	// Check for optional 'else' block
	if p.peekTokenIs(lexer.ELSE) {
		p.nextToken() // Consume 'else'

		if !p.expectPeek(lexer.LBRACE) { // Expect '{' for alternative block
			return nil
		}

		expression.Alternative = p.parseBlockStatement()
	}

	return expression
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

	// Check for empty list: call()
	if p.peekTokenIs(end) {
		p.nextToken() // Consume the end token (e.g., ')')
		return list
	}

	p.nextToken() // Consume '(' or ',' to get to the first expression
	list = append(list, p.parseExpression(LOWEST))

	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken() // Consume ','
		p.nextToken() // Consume the token starting the next expression
		list = append(list, p.parseExpression(LOWEST))
	}

	if !p.expectPeek(end) {
		return nil // Expected the end token (e.g., ')')
	}

	return list
}
