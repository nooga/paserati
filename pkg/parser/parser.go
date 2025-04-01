package parser

import (
	"fmt"
	"paserati/pkg/lexer"
	"strconv"
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
	p.registerPrefix(lexer.BANG, p.parsePrefixExpression)      // !true
	p.registerPrefix(lexer.MINUS, p.parsePrefixExpression)     // -5
	p.registerPrefix(lexer.INC, p.parsePrefixUpdateExpression) // Added ++x
	p.registerPrefix(lexer.DEC, p.parsePrefixUpdateExpression) // Added --x
	p.registerPrefix(lexer.LPAREN, p.parseGroupedExpression)   // (5 + 5)
	p.registerPrefix(lexer.IF, p.parseIfExpression)            // if (condition) { ... }

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
	debugPrint("nextToken(): cur='%s' (%s), peek='%s' (%s)", p.curToken.Literal, p.curToken.Type, p.peekToken.Literal, p.peekToken.Type)
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

	// Optional semicolon - Consume it here
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
		// We need curToken to be '=>' for parseArrowFunctionBodyAndFinish
		p.nextToken() // Consume the identifier token (which is curToken)
		debugPrint("parseIdentifier: Consumed IDENT, cur is now '%s' (%s)", p.curToken.Literal, p.curToken.Type)
		return p.parseArrowFunctionBodyAndFinish([]*Identifier{ident})
	}

	debugPrint("parseIdentifier: Just identifier '%s', returning.", ident.Value)
	return ident
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

// parseGroupedExpression handles expressions like (expr) OR arrow functions like () => expr or (a, b) => expr
func (p *Parser) parseGroupedExpression() Expression {
	startPos := p.l.CurrentPosition()
	startCur := p.curToken
	startPeek := p.peekToken
	startErrors := len(p.errors) // Track error count for backtracking
	debugPrint("parseGroupedExpression: Starting at pos %d, cur='%s', peek='%s'", startPos, startCur.Literal, startPeek.Literal)

	// --- Attempt to parse as Arrow Function Parameters ---
	// Check if curToken is '('
	if p.curTokenIs(lexer.LPAREN) {
		debugPrint("parseGroupedExpression: Attempting arrow param parse...")
		// Create a temporary params list - parseParameterList consumes tokens
		// We need to call it carefully. It expects to be called when cur=LPAREN.
		params := p.parseParameterList() // This consumes up to and including ')'

		// Check if param parsing succeeded AND if '=>' follows
		if params != nil && p.curTokenIs(lexer.RPAREN) && p.peekTokenIs(lexer.ARROW) {
			// Success! It's an arrow function.
			debugPrint("parseGroupedExpression: Successfully parsed arrow params: %v, found '=>' next.", params)
			p.nextToken() // Consume '=>', curToken is now '=>'
			debugPrint("parseGroupedExpression: Consumed '=>', cur='%s' (%s)", p.curToken.Literal, p.curToken.Type)
			// Remove any speculative errors added during param parsing try
			p.errors = p.errors[:startErrors]
			return p.parseArrowFunctionBodyAndFinish(params)
		} else {
			debugPrint("parseGroupedExpression: Failed arrow param parse (params=%v, cur='%s', peek='%s') or no '=>', backtracking...", params, p.curToken.Literal, p.peekToken.Type)
			// Backtrack: Restore lexer and parser state
			p.l.SetPosition(startPos) // Reset lexer position
			p.curToken = startCur
			p.peekToken = startPeek
			// Reset token state by calling nextToken twice (like in NewParser)
			// This is crucial to re-sync peekToken correctly after SetPosition.
			// p.nextToken()
			// p.nextToken()
			// Simpler alternative: just call nextToken once after setting cur/peek?
			// Let's try setting cur/peek and then letting the normal flow call nextToken.

			// Remove any errors added during the failed attempt
			p.errors = p.errors[:startErrors]
			debugPrint("parseGroupedExpression: Backtrack complete. cur='%s', peek='%s'", p.curToken.Literal, p.peekToken.Literal)
		}
	} else {
		debugPrint("parseGroupedExpression: Not starting with '(', cannot be parenthesized arrow params.")
	}

	// --- If not arrow function, parse as regular Grouped Expression ---
	debugPrint("parseGroupedExpression: Parsing as regular grouped expression.")
	if !p.curTokenIs(lexer.LPAREN) {
		// Should already be handled by prefix dispatch, but check defensively
		p.noPrefixParseFnError(lexer.LPAREN)
		return nil
	}
	p.nextToken() // Consume '('
	debugPrint("parseGroupedExpression: Consumed '(', cur='%s'", p.curToken.Literal)
	exp := p.parseExpression(LOWEST)
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
	debugPrint("parseIfExpression parsed condition: %s", expr.Condition.String())

	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}

	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	debugPrint("parseIfExpression parsing consequence block...")
	expr.Consequence = p.parseBlockStatement()
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
			expr.Alternative = p.parseBlockStatement()
			debugPrint("parseIfExpression parsed standard 'else' block.")
		} else {
			// Error: expected '{' or 'if' after 'else'
			msg := fmt.Sprintf("expected { or if after else, got %s instead at line %d", p.peekToken.Type, p.peekToken.Line)
			p.errors = append(p.errors, msg)
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

// parseArrowFunctionBodyAndFinish completes parsing an arrow function.
// It assumes the parameters have been parsed and the current token is '=>'.
func (p *Parser) parseArrowFunctionBodyAndFinish(params []*Identifier) Expression {
	debugPrint("parseArrowFunctionBodyAndFinish: Starting, curToken='%s' (%s), params=%v", p.curToken.Literal, p.curToken.Type, params)
	arrowFunc := &ArrowFunctionLiteral{
		Token:      p.curToken, // The '=>' token
		Parameters: params,
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
func (p *Parser) parseParameterList() []*Identifier {
	params := []*Identifier{}

	if !p.curTokenIs(lexer.LPAREN) { // Check current token IS LPAREN
		msg := fmt.Sprintf("line %d: internal error: parseParameterList called without current token being '(", p.curToken.Line)
		p.errors = append(p.errors, msg)
		debugPrint("parseParameterList: Error - %s", msg)
		return nil // Should not happen if called correctly
	}
	// No nextToken here, caller consumes '(', or we are already past it.
	// Let's assume the caller just ensures curToken is '(' before calling.
	// So, we look at peekToken immediately for ')' or the first param.
	debugPrint("parseParameterList: Starting, cur='%s', peek='%s'", p.curToken.Literal, p.peekToken.Literal)

	// Handle empty list: () => ...
	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken() // Consume ')'
		debugPrint("parseParameterList: Found empty list '()'")
		return params // Return empty slice
	}

	// Parse the first parameter
	p.nextToken() // Move to the first parameter identifier
	if !p.curTokenIs(lexer.IDENT) {
		msg := fmt.Sprintf("line %d: expected identifier as parameter, got %s", p.curToken.Line, p.curToken.Type)
		p.errors = append(p.errors, msg)
		debugPrint("parseParameterList: Error - %s", msg)
		return nil
	}
	firstParam := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	params = append(params, firstParam)
	debugPrint("parseParameterList: Parsed param '%s'", firstParam.Value)

	// Parse subsequent parameters (comma-separated)
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken() // Consume ','
		p.nextToken() // Consume identifier
		if !p.curTokenIs(lexer.IDENT) {
			msg := fmt.Sprintf("line %d: expected identifier after comma, got %s", p.curToken.Line, p.curToken.Type)
			p.errors = append(p.errors, msg)
			debugPrint("parseParameterList: Error - %s", msg)
			return nil
		}
		param := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		params = append(params, param)
		debugPrint("parseParameterList: Parsed param '%s'", param.Value)
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
	debugPrint("parseTernaryExpression parsed consequence: %s", expr.Consequence.String())

	if !p.expectPeek(lexer.COLON) {
		debugPrint("parseTernaryExpression failed: expected COLON")
		return nil // Error already added by expectPeek
	}

	p.nextToken() // Consume ':'

	// Parse the alternative expression
	debugPrint("parseTernaryExpression parsing alternative...")
	expr.Alternative = p.parseExpression(LOWEST) // Continue with low precedence
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

	// Basic Check: Ensure the left side is an identifier for now.
	// Later, could support member access (obj.prop = ...) or index access (arr[i] = ...).
	if _, ok := left.(*Identifier); !ok {
		msg := fmt.Sprintf("line %d: invalid left-hand side in assignment: expected identifier, got %T", p.curToken.Line, left)
		p.errors = append(p.errors, msg)
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
				letStmt.TypeAnnotation = p.parseIdentifier()
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
		msg := fmt.Sprintf("line %d: invalid argument for prefix %s: expected identifier, got %T",
			expr.Token.Line, expr.Operator, expr.Argument)
		p.errors = append(p.errors, msg)
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
		msg := fmt.Sprintf("line %d: invalid argument for postfix %s: expected identifier, got %T",
			expr.Token.Line, expr.Operator, expr.Argument)
		p.errors = append(p.errors, msg)
		return nil
	}

	// No need to consume token, parseExpression loop does that.
	return expr
}
