package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/source"
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
	source *source.SourceFile // cached from lexer
	errors []errors.PaseratiError

	curToken  lexer.Token
	peekToken lexer.Token

	// Pratt parser for VALUE expressions
	prefixParseFns map[lexer.TokenType]prefixParseFn
	infixParseFns  map[lexer.TokenType]infixParseFn

	// --- NEW: Pratt parser for TYPE expressions ---
	typePrefixParseFns map[lexer.TokenType]prefixParseFn // Handles starts of types (e.g., number, string, ident, (), [])
	typeInfixParseFns  map[lexer.TokenType]infixParseFn  // Handles type operators (e.g., |, &)

	// Context tracking
	inGenerator     int // Counter for nested generator contexts (0 = not in generator)
	inAsyncFunction int // Counter for nested async function contexts (0 = not in async function)
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
	COMMA         // , (very low precedence, but higher than LOWEST)
	ARG_SEPARATOR // Virtual precedence level for argument list parsing (between COMMA and ASSIGNMENT)
	ASSIGNMENT    // =, +=, -=, *=, /=, %=, **=, &=, |=, ^=, <<=, >>=, >>>=, &&=, ||=, ??=
	TERNARY       // ?:
	COALESCE      // ??
	LOGICAL_OR    // ||
	LOGICAL_AND   // &&
	BITWISE_OR    // |  (Lower than XOR)
	BITWISE_XOR   // ^  (Lower than AND)
	BITWISE_AND   // &  (Lower than Equality)
	EQUALS        // ==, !=, ===, !==
	LESSGREATER   // >, <, >=, <=
	SHIFT         // <<, >>, >>> (Lower than Add/Sub)
	SUM           // + or -
	PRODUCT       // * or / or %
	POWER         // ** (Right-associative handled in parseInfix)
	PREFIX        // -X or !X or ++X or --X or ~X
	POSTFIX       // X++ or X--
	ASSERTION     // value as Type
	CALL          // myFunction(X)
	INDEX         // array[index]
	MEMBER        // object.property
)

// --- NEW: Type Precedence ---
const (
	_ int = iota
	TYPE_LOWEST
	TYPE_CONDITIONAL  // extends ? : (Very low precedence - should be parsed last)
	TYPE_PREDICATE    // is (Lower precedence - should be parsed last)
	TYPE_UNION        // |
	TYPE_INTERSECTION // &  (Higher precedence than union)
	TYPE_ARRAY        // [] (Higher precedence than intersection)
	TYPE_MEMBER       // . (Highest precedence - member access)
)

// Precedences map for VALUE operator tokens
var precedences = map[lexer.TokenType]int{
	// Comma operator (Lowest precedence)
	lexer.COMMA: COMMA,
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

	// Prefix operators - need to be in precedence table to prevent incorrect parsing
	lexer.TYPEOF: PREFIX, // typeof operator (unary)
	lexer.VOID:   PREFIX, // void operator (unary)

	// Prefix/Postfix (Handled by registration, not just precedence map)
	// lexer.BANG does not need precedence here (uses PREFIX in parsePrefix)
	// lexer.BITWISE_NOT does not need precedence here (uses PREFIX in parsePrefix)
	// lexer.INC prefix/postfix handled by registration
	// lexer.DEC prefix/postfix handled by registration

	// Type Assertion
	lexer.AS:        ASSERTION,
	lexer.SATISFIES: ASSERTION,

	// Call, Index, Member Access
	lexer.LPAREN:            CALL,
	lexer.LBRACKET:          INDEX,
	lexer.DOT:               MEMBER,
	lexer.TEMPLATE_START:    CALL,   // Tagged template: tag`...`
	lexer.OPTIONAL_CHAINING: MEMBER, // Same precedence as regular member access

	// Postfix operators need precedence for the parseExpression loop termination condition
	lexer.INC:  POSTFIX,
	lexer.DEC:  POSTFIX,
	lexer.BANG: POSTFIX, // Non-null assertion: x!
}

// --- NEW: Precedences map for TYPE operator tokens ---
var typePrecedences = map[lexer.TokenType]int{
	lexer.EXTENDS:     TYPE_CONDITIONAL,
	lexer.IS:          TYPE_PREDICATE,
	lexer.PIPE:        TYPE_UNION,
	lexer.BITWISE_AND: TYPE_INTERSECTION,
	lexer.LBRACKET:    TYPE_ARRAY,
	lexer.DOT:         TYPE_MEMBER,
}

// NewParser creates a new Parser.
func NewParser(l *lexer.Lexer) *Parser {
	p := &Parser{
		l:      l,
		source: l.GetSource(), // Cache source from lexer
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
	p.registerPrefix(lexer.BIGINT, p.parseBigIntLiteral)
	p.registerPrefix(lexer.STRING, p.parseStringLiteral)
	p.registerPrefix(lexer.REGEX_LITERAL, p.parseRegexLiteral)     // NEW: Regex literals
	p.registerPrefix(lexer.TEMPLATE_START, p.parseTemplateLiteral) // NEW: Template literals
	p.registerPrefix(lexer.TRUE, p.parseBooleanLiteral)
	p.registerPrefix(lexer.FALSE, p.parseBooleanLiteral)
	p.registerPrefix(lexer.NULL, p.parseNullLiteral)
	p.registerPrefix(lexer.UNDEFINED, p.parseUndefinedLiteral)  // Keep for value context
	p.registerPrefix(lexer.THIS, p.parseThisExpression)         // Added for this keyword
	p.registerPrefix(lexer.NEW, p.parseNewExpression)           // Added for new keyword
	p.registerPrefix(lexer.IMPORT, p.parseImportMetaExpression) // Added for import.meta
	p.registerPrefix(lexer.GET, p.parseIdentifier)              // GET can be used as identifier in object literals
	p.registerPrefix(lexer.SET, p.parseIdentifier)              // SET can be used as identifier in object literals
	p.registerPrefix(lexer.THROW, p.parseIdentifier)            // THROW can be used as identifier in object literals
	p.registerPrefix(lexer.RETURN, p.parseIdentifier)           // RETURN can be used as identifier in object literals
	p.registerPrefix(lexer.LET, p.parseIdentifier)              // LET can be used as identifier in non-strict mode
	// FutureReservedWords - can be used as identifiers in non-strict mode
	p.registerPrefix(lexer.STATIC, p.parseIdentifier)
	p.registerPrefix(lexer.IMPLEMENTS, p.parseIdentifier)
	p.registerPrefix(lexer.INTERFACE, p.parseIdentifier)
	p.registerPrefix(lexer.PRIVATE, p.parseIdentifier)
	p.registerPrefix(lexer.PROTECTED, p.parseIdentifier)
	p.registerPrefix(lexer.PUBLIC, p.parseIdentifier)
	p.registerPrefix(lexer.OF, p.parseIdentifier)   // OF is a contextual keyword, can be used as identifier
	p.registerPrefix(lexer.FROM, p.parseIdentifier) // FROM is a contextual keyword (import/export), can be used as identifier
	p.registerPrefix(lexer.TYPE, p.parseIdentifier) // TYPE is a contextual keyword (TypeScript), can be used as identifier in JS
	p.registerPrefix(lexer.FUNCTION, p.parseFunctionLiteral)
	p.registerPrefix(lexer.ASYNC, p.parseAsyncExpression) // Added for async functions and async arrows
	p.registerPrefix(lexer.CLASS, p.parseClassExpression)
	p.registerPrefix(lexer.BANG, p.parsePrefixExpression)
	p.registerPrefix(lexer.MINUS, p.parsePrefixExpression)
	p.registerPrefix(lexer.PLUS, p.parsePrefixExpression) // Added for unary plus
	p.registerPrefix(lexer.BITWISE_NOT, p.parsePrefixExpression)
	p.registerPrefix(lexer.TYPEOF, p.parseTypeofExpression) // Added for typeof operator
	p.registerPrefix(lexer.VOID, p.parseVoidExpression)     // Added for void operator
	p.registerPrefix(lexer.DELETE, p.parsePrefixExpression) // Added for delete operator
	p.registerPrefix(lexer.YIELD, p.parseYieldExpression)   // Added for yield expressions
	p.registerPrefix(lexer.AWAIT, p.parseAwaitExpression)   // Added for await expressions
	p.registerPrefix(lexer.INC, p.parsePrefixUpdateExpression)
	p.registerPrefix(lexer.DEC, p.parsePrefixUpdateExpression)
	p.registerPrefix(lexer.LPAREN, p.parseGroupedExpression)
	p.registerPrefix(lexer.LT, p.parseGenericArrowFunction) // Generic arrow functions: <T>(x: T) => x
	p.registerPrefix(lexer.IF, p.parseIfExpression)
	p.registerPrefix(lexer.LBRACKET, p.parseArrayLiteral)      // Value context: Array literal
	p.registerPrefix(lexer.LBRACE, p.parseObjectLiteral)       // <<< NEW: Register Object Literal Parsing
	p.registerPrefix(lexer.SPREAD, p.parseSpreadElement)       // NEW: Spread syntax in calls
	p.registerPrefix(lexer.SUPER, p.parseSuperExpression)      // NEW: Super expressions
	p.registerPrefix(lexer.PRIVATE_IDENT, p.parsePrivateIdent) // Private field presence check: #field in obj

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
	p.registerInfix(lexer.LT, p.parseGenericCallOrComparison)
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
	p.registerInfix(lexer.SATISFIES, p.parseSatisfiesExpression)

	// Call, Index, Member, Ternary
	p.registerInfix(lexer.LPAREN, p.parseCallExpression)    // Value context: function call
	p.registerInfix(lexer.LBRACKET, p.parseIndexExpression) // Value context: array/member index
	p.registerInfix(lexer.DOT, p.parseMemberExpression)
	p.registerInfix(lexer.TEMPLATE_START, p.parseTaggedTemplateInfix) // tag`...`
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
	// Non-null assertion operator (x!)
	p.registerInfix(lexer.BANG, p.parseNonNullExpression)
	// Comma operator
	p.registerInfix(lexer.COMMA, p.parseCommaExpression)

	// --- Register TYPE Prefix Functions ---
	// --- MODIFIED: Use parseTypeIdentifier for simple type names ---
	p.registerTypePrefix(lexer.IDENT, p.parseTypeIdentifier)               // Basic types like 'number', 'string', custom types
	p.registerTypePrefix(lexer.NULL, p.parseNullLiteral)                   // 'null' type
	p.registerTypePrefix(lexer.UNDEFINED, p.parseUndefinedLiteral)         // 'undefined' type
	p.registerTypePrefix(lexer.VOID, p.parseVoidTypeLiteral)               // 'void' type
	p.registerTypePrefix(lexer.KEYOF, p.parseKeyofTypeExpression)          // 'keyof' type operator
	p.registerTypePrefix(lexer.TYPEOF, p.parseTypeofTypeExpression)        // 'typeof' type operator
	p.registerTypePrefix(lexer.INFER, p.parseInferTypeExpression)          // 'infer' type operator
	p.registerTypePrefix(lexer.TEMPLATE_START, p.parseTemplateLiteralType) // Template literal types
	// NEW: Constructor types that start with 'new'
	p.registerTypePrefix(lexer.NEW, p.parseConstructorTypeExpression) // NEW: Constructor types like 'new () => T'
	// Literal types in TYPE context too
	p.registerTypePrefix(lexer.STRING, p.parseStringLiteral)
	p.registerTypePrefix(lexer.NUMBER, p.parseNumberLiteral)
	p.registerTypePrefix(lexer.BIGINT, p.parseBigIntLiteral)
	p.registerTypePrefix(lexer.TRUE, p.parseBooleanLiteral)
	p.registerTypePrefix(lexer.FALSE, p.parseBooleanLiteral)
	// Function types that start with '('
	p.registerTypePrefix(lexer.LPAREN, p.parseFunctionTypeExpression) // Starts with '(', e.g., '() => number'
	// Generic function types that start with '<'
	p.registerTypePrefix(lexer.LT, p.parseGenericFunctionTypeExpression) // Starts with '<', e.g., '<T>(x: T) => T'
	// Object type literals that start with '{'
	p.registerTypePrefix(lexer.LBRACE, p.parseObjectTypeExpression) // NEW: Object type literals like { name: string; age: number }
	// --- NEW: Tuple type literals that start with '[' ---
	p.registerTypePrefix(lexer.LBRACKET, p.parseTupleTypeExpression) // NEW: Tuple type literals like [string, number, boolean?]
	// --- NEW: Leading pipe union types that start with '|' ---
	p.registerTypePrefix(lexer.PIPE, p.parseLeadingPipeUnionType) // NEW: Leading pipe union types like | A | B

	// --- Register TYPE Infix Functions ---
	p.registerTypeInfix(lexer.PIPE, p.parseUnionTypeExpression)               // TYPE context: '|' is union
	p.registerTypeInfix(lexer.BITWISE_AND, p.parseIntersectionTypeExpression) // TYPE context: '&' is intersection
	p.registerTypeInfix(lexer.LBRACKET, p.parseArrayTypeExpression)           // TYPE context: 'T[]'
	p.registerTypeInfix(lexer.IS, p.parseTypePredicateExpression)             // TYPE context: 'x is Type' for type predicates
	p.registerTypeInfix(lexer.EXTENDS, p.parseConditionalTypeExpression)      // TYPE context: 'T extends U ? X : Y'
	p.registerTypeInfix(lexer.DOT, p.parseEnumMemberTypeExpression)           // TYPE context: 'EnumName.MemberName'

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

// expectPeekGT is like expectPeek(GT) but handles >>, >>>, >>=, and >= in generic contexts
// If peek is >> or >>>, it splits the token and consumes one >
func (p *Parser) expectPeekGT() bool {
	if p.peekToken.Type == lexer.GT {
		p.nextToken()
		return true
	} else if p.peekToken.Type == lexer.RIGHT_SHIFT {
		// Split >> into > and > using lexer's split method
		splitToken := p.l.SplitRightShiftToken(p.peekToken)
		// Update peek to be the first > and advance
		p.peekToken = splitToken
		p.nextToken() // This will make current = first >, peek = second >
		return true
	} else if p.peekToken.Type == lexer.UNSIGNED_RIGHT_SHIFT {
		// Split >>> into > and >> using lexer's split method
		splitToken := p.l.SplitUnsignedRightShiftToken(p.peekToken)
		// Update peek to be the first > and advance
		p.peekToken = splitToken
		p.nextToken() // This will make current = first >, peek = >>
		return true
	} else if p.peekToken.Type == lexer.RIGHT_SHIFT_ASSIGN {
		// Split >>= into > and >= using lexer's split method
		// This handles cases like: Array<Array<T>> = []
		splitToken := p.l.SplitRightShiftAssignToken(p.peekToken)
		// Update peek to be the first > and advance
		p.peekToken = splitToken
		p.nextToken() // This will make current = first >, peek = >=
		return true
	} else if p.peekToken.Type == lexer.GE {
		// Split >= into > and = using lexer's split method
		// This handles deeply nested generics like: Array<Array<Array<T>>> = []
		splitToken := p.l.SplitGreaterEqualToken(p.peekToken)
		// Update peek to be the first > and advance
		p.peekToken = splitToken
		p.nextToken() // This will make current = first >, peek = =
		return true
	}

	p.peekError(lexer.GT)
	return false
}

// ParseProgram parses the entire input and returns the root Program node and any errors.
func (p *Parser) ParseProgram() (*Program, []errors.PaseratiError) {
	program := &Program{}
	program.Statements = []Statement{}
	program.HoistedDeclarations = make(map[string]Expression) // Initialize map with Expression
	program.Source = p.source                                 // Set source context for error reporting

	for p.curToken.Type != lexer.EOF {
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)

			// --- Hoisting Check ---
			// Check if the statement IS an ExpressionStatement containing a FunctionLiteral
			if exprStmt, isExprStmt := stmt.(*ExpressionStatement); isExprStmt && exprStmt != nil {
				if exprStmt.Expression != nil {
					if funcLit, isFuncLit := exprStmt.Expression.(*FunctionLiteral); isFuncLit && funcLit.Name != nil {
						// In JavaScript, duplicate function declarations are allowed
						// The last declaration wins due to hoisting
						program.HoistedDeclarations[funcLit.Name.Value] = funcLit // Store Expression
					}
				}
			}

			// Also check for exported functions
			if exportDecl, isExport := stmt.(*ExportNamedDeclaration); isExport && exportDecl != nil && exportDecl.Declaration != nil {
				// Check if the exported declaration is a function
				if exprStmt, isExprStmt := exportDecl.Declaration.(*ExpressionStatement); isExprStmt && exprStmt != nil {
					if exprStmt.Expression != nil {
						if funcLit, isFuncLit := exprStmt.Expression.(*FunctionLiteral); isFuncLit && funcLit.Name != nil {
							// In JavaScript, duplicate function declarations are allowed
							// The last declaration wins due to hoisting
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
	debugPrint("parseStatement: cur='%s' (%s), peek='%s' (%s)", p.curToken.Literal, p.curToken.Type, p.peekToken.Literal, p.peekToken.Type)
	switch p.curToken.Type {
	case lexer.LET:
		// Check if this is actually a let declaration or just 'let' as an identifier
		// If followed by [, {, or identifier, it's a declaration
		// Otherwise (e.g., followed by ;, ), }, etc.), treat as identifier
		if p.peekTokenIs(lexer.LBRACKET) || p.peekTokenIs(lexer.LBRACE) || p.peekTokenIs(lexer.IDENT) ||
			p.isKeywordThatCanBeIdentifier(p.peekToken.Type) {
			return p.parseLetStatement()
		} else {
			// Treat 'let' as an identifier in an expression statement
			return p.parseExpressionStatement()
		}
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
	case lexer.WITH:
		return p.parseWithStatement()
	case lexer.BREAK:
		return p.parseBreakStatement()
	case lexer.SEMICOLON:
		return p.parseEmptyStatement()
	case lexer.CONTINUE:
		return p.parseContinueStatement()
	case lexer.TYPE:
		return p.parseTypeAliasStatement()
	case lexer.INTERFACE:
		return p.parseInterfaceDeclaration()
	case lexer.SWITCH:
		return p.parseSwitchStatement()
	case lexer.FUNCTION:
		stmt := p.parseFunctionDeclarationStatement()
		return stmt
	case lexer.ASYNC:
		return p.parseAsyncFunctionDeclarationStatement()
	case lexer.CLASS:
		return p.parseClassDeclarationStatement()
	case lexer.ENUM:
		return p.parseEnumDeclarationStatement()
	case lexer.ABSTRACT:
		return p.parseAbstractClassDeclarationStatement()
	case lexer.TRY:
		return p.parseTryStatement()
	case lexer.THROW:
		return p.parseThrowStatement()
	case lexer.IMPORT:
		// Check if this is import.meta, import(), or import declaration
		if p.peekTokenIs(lexer.DOT) {
			// This is import.meta, parse as expression statement
			return p.parseExpressionStatement()
		}
		if p.peekTokenIs(lexer.LPAREN) {
			// This is import(), parse as expression statement (dynamic import)
			return p.parseExpressionStatement()
		}
		return p.parseImportDeclaration()
	case lexer.EXPORT:
		return p.parseExportDeclaration()
	case lexer.LBRACE:
		// Check if this is a block statement or destructuring assignment
		// Look ahead to see if this might be destructuring
		if p.isDestructuringAssignment() {
			return p.parseExpressionStatement()
		}
		return p.parseBlockStatement()
	case lexer.RBRACE:
		// End of current block scope; let the caller handle it
		return nil
	case lexer.IDENT:
		// Check if this is a labeled statement (identifier followed by colon)
		if p.peekTokenIs(lexer.COLON) {
			return p.parseLabeledStatement()
		}
		return p.parseExpressionStatement()
	case lexer.YIELD:
		// In non-strict mode and outside generators, yield can be used as a label
		if p.peekTokenIs(lexer.COLON) && p.inGenerator == 0 {
			return p.parseLabeledStatement()
		}
		return p.parseExpressionStatement()
	case lexer.AWAIT:
		// In non-strict mode and outside async functions, await can be used as a label
		if p.peekTokenIs(lexer.COLON) && p.inAsyncFunction == 0 {
			return p.parseLabeledStatement()
		}
		return p.parseExpressionStatement()
	case lexer.ILLEGAL:
		// Handle ILLEGAL tokens by adding error and advancing
		p.addError(p.curToken, fmt.Sprintf("illegal token: %s", p.curToken.Literal))
		p.nextToken() // Advance past the ILLEGAL token to avoid infinite loop
		return nil
	default:
		return p.parseExpressionStatement()
	}
}

// isDestructuringAssignment checks if the current { starts a destructuring assignment.
// Per ECMAScript, at the statement level, { always starts a block statement.
// Destructuring assignments at statement level MUST be parenthesized: ({ a } = x);
// So at statement level, we should NEVER treat a bare { as destructuring.
// This function is called from parseStatement, so we should return false.
func (p *Parser) isDestructuringAssignment() bool {
	// At statement level, { always starts a block statement per ECMAScript grammar.
	// Destructuring assignments like { a } = x require parentheses at statement level.
	// The parenthesized form ({ a } = x) is handled as an expression statement.
	return false
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

// --- Async Function Declaration Statement Parsing ---
func (p *Parser) parseAsyncFunctionDeclarationStatement() *ExpressionStatement {
	// We're at the 'async' token, peek should be 'function'
	if !p.expectPeek(lexer.FUNCTION) {
		return &ExpressionStatement{
			Token:      p.curToken,
			Expression: nil,
		}
	}

	// Parse the function as an expression (FunctionLiteral with IsAsync=true)
	funcExpr := p.parseFunctionLiteral()
	if funcExpr == nil {
		return &ExpressionStatement{
			Token:      p.curToken,
			Expression: nil,
		}
	}

	// Mark it as async
	if funcLit, ok := funcExpr.(*FunctionLiteral); ok {
		funcLit.IsAsync = true
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

// --- Class Declaration Statement Parsing ---
func (p *Parser) parseClassDeclarationStatement() Statement {
	// Parse the class as a proper declaration statement
	return p.parseClassDeclaration()
}

func (p *Parser) parseAbstractClassDeclarationStatement() Statement {
	// We're at the 'abstract' token, peek should be 'class'
	if !p.expectPeek(lexer.CLASS) {
		return &ExpressionStatement{
			Token:      p.curToken,
			Expression: nil,
		}
	}

	// Parse the class as a declaration (ClassDeclaration)
	classDeclStmt := p.parseClassDeclaration()
	if classDeclStmt == nil {
		return &ExpressionStatement{
			Token:      p.curToken,
			Expression: nil,
		}
	}

	// Mark the class as abstract
	if classDecl, ok := classDeclStmt.(*ClassDeclaration); ok {
		classDecl.IsAbstract = true
	}

	// Optional semicolon
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return classDeclStmt
}

// --- NEW: Type Alias Statement Parsing ---
func (p *Parser) parseTypeAliasStatement() *TypeAliasStatement {
	stmt := &TypeAliasStatement{Token: p.curToken} // 'type' token

	if !p.expectPeek(lexer.IDENT) {
		return nil // Expected identifier after 'type'
	}

	stmt.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check for type parameters (same pattern as functions)
	stmt.TypeParameters = p.tryParseTypeParameters()

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
// Also handles parenthesized types like (T) or ((T) => U)[] where the parens group a type.
func (p *Parser) parseFunctionTypeExpression() Expression {
	startToken := p.curToken // '(' token

	// Try to parse as function type parameter list
	var parseErr error
	params, restParam, parseErr := p.parseFunctionTypeParameterList()
	if parseErr != nil {
		// Error already added by helper
		return nil
	}

	// Check if this is a function type (followed by '=>') or a parenthesized type
	if p.peekTokenIs(lexer.ARROW) {
		// This is a function type: (params) => returnType
		funcType := &FunctionTypeExpression{Token: startToken}
		funcType.Parameters = params
		funcType.RestParameter = restParam

		p.nextToken() // Consume '=>'
		p.nextToken() // Move to the return type
		funcType.ReturnType = p.parseTypeExpression()
		if funcType.ReturnType == nil {
			return nil // Error parsing return type
		}

		return funcType
	}

	// Not followed by '=>', so this is a parenthesized type: (T)
	// The "params" should contain exactly one type expression
	if len(params) != 1 || restParam != nil {
		// Invalid: parenthesized type must contain exactly one type
		p.addError(startToken, "parenthesized type must contain exactly one type, or use '=>' for function type")
		return nil
	}

	// Return the inner type - the caller (parseTypeExpressionRecursive) will handle
	// any suffix like [] for array types
	return params[0]
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
	if p.curTokenIs(lexer.IDENT) {
		if p.peekTokenIs(lexer.QUESTION) {
			// Optional parameter: name?: type
			p.nextToken() // Consume IDENT
			p.nextToken() // Consume '?'
			if !p.curTokenIs(lexer.COLON) {
				return nil, nil, fmt.Errorf("expected ':' after '?' in optional parameter")
			}
			p.nextToken() // Move to the actual type
		} else if p.peekTokenIs(lexer.COLON) {
			// Required parameter: name: type
			p.nextToken() // Consume IDENT
			p.nextToken() // Consume ':', move to the actual type
		}
		// else: just a type without parameter name
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

		// Handle trailing comma - if we see ')' after a comma, we're done
		if p.curTokenIs(lexer.RPAREN) {
			// This is a trailing comma, we're already at the closing paren
			// Just return without expecting another RPAREN
			return params, restParam, nil
		}

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
		if p.curTokenIs(lexer.IDENT) {
			if p.peekTokenIs(lexer.QUESTION) {
				// Optional parameter: name?: type
				p.nextToken() // Consume IDENT
				p.nextToken() // Consume '?'
				if !p.curTokenIs(lexer.COLON) {
					return nil, nil, fmt.Errorf("expected ':' after '?' in optional parameter")
				}
				p.nextToken() // Move to the actual type
			} else if p.peekTokenIs(lexer.COLON) {
				// Required parameter: name: type
				p.nextToken() // Consume IDENT
				p.nextToken() // Consume ':', move to the actual type
			}
			// else: just a type without parameter name
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

// --- NEW: Helper for conditional type parsing ---
// This function handles conditional types like T extends U ? X : Y
func (p *Parser) parseConditionalTypeExpression(left Expression) Expression {
	conditionalExp := &ConditionalTypeExpression{
		ExtendsToken: p.curToken, // The 'extends' token
		CheckType:    left,       // The left side is the type being checked
	}

	// Parse the type after 'extends'
	precedence := TYPE_CONDITIONAL
	p.nextToken() // Consume the token starting the extends type
	conditionalExp.ExtendsType = p.parseTypeExpressionRecursive(precedence)
	if conditionalExp.ExtendsType == nil {
		return nil // Error parsing extends type
	}

	// Expect '?' token
	if !p.expectPeek(lexer.QUESTION) {
		return nil
	}
	conditionalExp.QuestionToken = p.curToken

	// Parse the true type
	p.nextToken() // Consume the token starting the true type
	conditionalExp.TrueType = p.parseTypeExpressionRecursive(precedence)
	if conditionalExp.TrueType == nil {
		return nil // Error parsing true type
	}

	// Expect ':' token
	if !p.expectPeek(lexer.COLON) {
		return nil
	}
	conditionalExp.ColonToken = p.curToken

	// Parse the false type
	p.nextToken() // Consume the token starting the false type
	conditionalExp.FalseType = p.parseTypeExpressionRecursive(precedence)
	if conditionalExp.FalseType == nil {
		return nil // Error parsing false type
	}

	return conditionalExp
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
	// This function handles both T[] (array types) and T[K] (indexed access types)
	lbracketToken := p.curToken // Save the '[' token

	// Check if this is an empty array type T[] or indexed access T[K]
	if p.peekTokenIs(lexer.RBRACKET) {
		// This is an array type T[]
		arrayTypeExp := &ArrayTypeExpression{
			Token:       lbracketToken,
			ElementType: elementType,
		}
		p.nextToken() // Consume ']'
		return arrayTypeExp
	} else {
		// This is an indexed access type T[K]
		indexedAccessExp := &IndexedAccessTypeExpression{
			Token:      lbracketToken,
			ObjectType: elementType,
		}

		// Parse the index type between brackets
		p.nextToken() // Move past '['
		indexedAccessExp.IndexType = p.parseTypeExpression()
		if indexedAccessExp.IndexType == nil {
			return nil // Error parsing index type
		}

		// Expect closing bracket
		if !p.expectPeek(lexer.RBRACKET) {
			return nil // Expected ']' after index type
		}

		return indexedAccessExp
	}
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

func (p *Parser) parseLetStatement() Statement {
	letToken := p.curToken // Save the 'let' token

	// Peek at the next token to determine if it's a destructuring pattern
	p.nextToken() // Move to what comes after 'let'

	switch p.curToken.Type {
	case lexer.LBRACKET:
		// Array destructuring: let [a, b] = ...
		return p.parseArrayDestructuringDeclaration(letToken, false, true)
	case lexer.LBRACE:
		// Object destructuring: let {a, b} = ...
		return p.parseObjectDestructuringDeclaration(letToken, false, true)
	case lexer.IDENT, lexer.YIELD, lexer.GET, lexer.SET, lexer.THROW, lexer.RETURN, lexer.LET, lexer.AWAIT,
		lexer.STATIC, lexer.IMPLEMENTS, lexer.INTERFACE, lexer.PRIVATE, lexer.PROTECTED, lexer.PUBLIC, lexer.OF, lexer.FROM:
		// Regular identifier: let x = ... or let x = ..., y = ... (including contextual keywords and FutureReservedWords)
		stmt := &LetStatement{Token: letToken}
		firstDeclarator := &VarDeclarator{}
		firstDeclarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

		// Optional Type Annotation
		if p.peekTokenIs(lexer.COLON) {
			p.nextToken() // Consume ':'
			p.nextToken() // Consume token starting the type expression
			firstDeclarator.TypeAnnotation = p.parseTypeExpression()
			if firstDeclarator.TypeAnnotation == nil {
				return nil
			}
		}

		// Allow omitting = value, defaulting to undefined
		if p.peekTokenIs(lexer.ASSIGN) {
			p.nextToken() // Consume '='
			p.nextToken() // Consume token starting the expression
			// Use COMMA precedence to allow assignment expressions but stop at commas
			// This enables: var result = [x, y] = [1, 2] while stopping at: var a = 1, b = 2
			firstDeclarator.Value = p.parseExpression(COMMA)
		}

		stmt.Declarations = []*VarDeclarator{firstDeclarator}

		// Parse additional declarations separated by commas
		for p.peekTokenIs(lexer.COMMA) {
			p.nextToken() // Consume ','

			if !p.expectPeekIdentifierOrKeyword() {
				return nil
			}

			declarator := &VarDeclarator{}
			declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

			// Optional Type Annotation
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken() // Consume ':'
				p.nextToken() // Consume token starting the type expression
				declarator.TypeAnnotation = p.parseTypeExpression()
				if declarator.TypeAnnotation == nil {
					return nil
				}
			}

			// Allow omitting = value, defaulting to undefined
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken() // Consume '='
				p.nextToken() // Consume token starting the expression
				// Use COMMA precedence to allow assignment expressions but stop at commas
				declarator.Value = p.parseExpression(COMMA)
			}

			stmt.Declarations = append(stmt.Declarations, declarator)
		}

		// Set legacy fields for backward compatibility (first declaration)
		if len(stmt.Declarations) > 0 {
			stmt.Name = stmt.Declarations[0].Name
			stmt.TypeAnnotation = stmt.Declarations[0].TypeAnnotation
			stmt.Value = stmt.Declarations[0].Value
			stmt.ComputedType = stmt.Declarations[0].ComputedType
		}

		// Optional semicolon
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken()
		}

		return stmt
	default:
		p.addError(p.curToken, fmt.Sprintf("expected identifier or destructuring pattern after 'let', got %s", p.curToken.Type))
		return nil
	}
}

func (p *Parser) parseConstStatement() Statement {
	constToken := p.curToken // Save the 'const' token

	// Peek at the next token to determine if it's a destructuring pattern
	p.nextToken() // Move to what comes after 'const'

	switch p.curToken.Type {
	case lexer.ENUM:
		// Const enum: const enum Name { ... }
		return p.parseConstEnumDeclarationStatement(constToken)
	case lexer.LBRACKET:
		// Array destructuring: const [a, b] = ...
		return p.parseArrayDestructuringDeclaration(constToken, true, true)
	case lexer.LBRACE:
		// Object destructuring: const {a, b} = ...
		return p.parseObjectDestructuringDeclaration(constToken, true, true)
	case lexer.IDENT, lexer.YIELD, lexer.GET, lexer.SET, lexer.THROW, lexer.RETURN, lexer.LET, lexer.AWAIT,
		lexer.STATIC, lexer.IMPLEMENTS, lexer.INTERFACE, lexer.PRIVATE, lexer.PROTECTED, lexer.PUBLIC, lexer.OF, lexer.FROM:
		// Regular identifier: const x = ... or const x = ..., y = ... (including contextual keywords and FutureReservedWords)
		stmt := &ConstStatement{Token: constToken}
		firstDeclarator := &VarDeclarator{}
		firstDeclarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

		// Optional Type Annotation
		if p.peekTokenIs(lexer.COLON) {
			p.nextToken() // Consume ':'
			p.nextToken() // Consume the token starting the type expression
			firstDeclarator.TypeAnnotation = p.parseTypeExpression()
			if firstDeclarator.TypeAnnotation == nil {
				return nil
			}
		}

		// const requires initializer
		if !p.expectPeek(lexer.ASSIGN) {
			return nil
		}

		p.nextToken()                                         // Consume token starting the expression
		firstDeclarator.Value = p.parseExpression(ASSIGNMENT) // Use ASSIGNMENT precedence to stop at comma

		stmt.Declarations = []*VarDeclarator{firstDeclarator}

		// Parse additional declarations separated by commas
		for p.peekTokenIs(lexer.COMMA) {
			p.nextToken() // Consume ','

			if !p.expectPeekIdentifierOrKeyword() {
				return nil
			}

			declarator := &VarDeclarator{}
			declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

			// Optional Type Annotation
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken() // Consume ':'
				p.nextToken() // Consume token starting the type expression
				declarator.TypeAnnotation = p.parseTypeExpression()
				if declarator.TypeAnnotation == nil {
					return nil
				}
			}

			// const requires initializer for each declarator
			if !p.expectPeek(lexer.ASSIGN) {
				return nil
			}

			p.nextToken()                                    // Consume token starting the expression
			declarator.Value = p.parseExpression(ASSIGNMENT) // Use ASSIGNMENT precedence to stop at comma

			stmt.Declarations = append(stmt.Declarations, declarator)
		}

		// Set legacy fields for backward compatibility (first declaration)
		if len(stmt.Declarations) > 0 {
			stmt.Name = stmt.Declarations[0].Name
			stmt.TypeAnnotation = stmt.Declarations[0].TypeAnnotation
			stmt.Value = stmt.Declarations[0].Value
			stmt.ComputedType = stmt.Declarations[0].ComputedType
		}

		// Optional semicolon
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken()
		}

		return stmt
	default:
		p.addError(p.curToken, fmt.Sprintf("expected identifier or destructuring pattern after 'const', got %s", p.curToken.Type))
		return nil
	}
}

func (p *Parser) parseVarStatement() Statement {
	varToken := p.curToken // Save the 'var' token

	// Peek at the next token to determine if it's a destructuring pattern
	p.nextToken() // Move to what comes after 'var'

	switch p.curToken.Type {
	case lexer.LBRACKET:
		// Array destructuring: var [a, b] = ...
		return p.parseArrayDestructuringDeclaration(varToken, false, true)
	case lexer.LBRACE:
		// Object destructuring: var {a, b} = ...
		debugPrint("// [PARSER DEBUG] parseVarStatement: detected LBRACE, calling parseObjectDestructuringDeclaration\n")
		return p.parseObjectDestructuringDeclaration(varToken, false, true)
	case lexer.IDENT, lexer.YIELD, lexer.GET, lexer.SET, lexer.THROW, lexer.RETURN, lexer.LET, lexer.AWAIT,
		lexer.STATIC, lexer.IMPLEMENTS, lexer.INTERFACE, lexer.PRIVATE, lexer.PROTECTED, lexer.PUBLIC, lexer.OF,
		lexer.UNDEFINED, lexer.NULL, lexer.FROM:
		// Regular identifier case (including contextual keywords, FutureReservedWords in non-strict mode,
		// and global property names like undefined/null which are not reserved words)
		stmt := &VarStatement{Token: varToken}
		firstDeclarator := &VarDeclarator{}
		firstDeclarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

		// Optional Type Annotation
		if p.peekTokenIs(lexer.COLON) {
			p.nextToken() // Consume ':'
			p.nextToken() // Consume token starting the type expression
			firstDeclarator.TypeAnnotation = p.parseTypeExpression()
			if firstDeclarator.TypeAnnotation == nil {
				return nil
			}
		}

		// Allow omitting = value, defaulting to undefined
		if p.peekTokenIs(lexer.ASSIGN) {
			p.nextToken() // Consume '='
			p.nextToken() // Consume token starting the expression
			// Use COMMA precedence to allow assignment expressions but stop at commas
			// This enables: var result = [x, y] = [1, 2] while stopping at: var a = 1, b = 2
			firstDeclarator.Value = p.parseExpression(COMMA)
		}

		stmt.Declarations = []*VarDeclarator{firstDeclarator}

		// Parse additional declarations separated by commas
		for p.peekTokenIs(lexer.COMMA) {
			p.nextToken() // Consume ','

			if !p.expectPeekIdentifierOrKeyword() {
				return nil
			}

			declarator := &VarDeclarator{}
			declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

			// Optional Type Annotation
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken() // Consume ':'
				p.nextToken() // Consume token starting the type expression
				declarator.TypeAnnotation = p.parseTypeExpression()
				if declarator.TypeAnnotation == nil {
					return nil
				}
			}

			// Allow omitting = value, defaulting to undefined
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken() // Consume '='
				p.nextToken() // Consume token starting the expression
				// Use COMMA precedence to allow assignment expressions but stop at commas
				declarator.Value = p.parseExpression(COMMA)
			}

			stmt.Declarations = append(stmt.Declarations, declarator)
		}

		// Set legacy fields for backward compatibility (first declaration)
		if len(stmt.Declarations) > 0 {
			stmt.Name = stmt.Declarations[0].Name
			stmt.TypeAnnotation = stmt.Declarations[0].TypeAnnotation
			stmt.Value = stmt.Declarations[0].Value
			stmt.ComputedType = stmt.Declarations[0].ComputedType
		}

		// Optional semicolon - Consume it here
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken()
		}

		return stmt
	default:
		p.addError(p.curToken, fmt.Sprintf("expected identifier or destructuring pattern after 'var', got %s", p.curToken.Type))
		return nil
	}
}

func (p *Parser) parseReturnStatement() *ReturnStatement {
	stmt := &ReturnStatement{Token: p.curToken}
	returnLine := p.curToken.Line
	p.nextToken() // Consume 'return'

	// ASI: If there's a line terminator after 'return', treat as 'return;'
	// This is a restricted production in ECMAScript
	if p.curToken.Line != returnLine {
		// Line terminator after 'return' - ASI inserts semicolon
		stmt.ReturnValue = nil
		return stmt
	}

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
	stmt.Condition = p.parseExpression(COMMA)
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

	// Optional semicolon - consume if next
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

	for !p.peekTokenIs(lexer.SEMICOLON) && !p.curTokenIs(lexer.SEMICOLON) && precedence < p.peekPrecedence() {
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
		return p.parseArrowFunctionBodyAndFinish(nil, []*Parameter{param}, nil, nil)
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
			// Check if it's an overflow - Go returns +Inf/-Inf with a range error
			// JavaScript represents overflows as Infinity, so this is valid
			if numErr, ok := err.(*strconv.NumError); ok && numErr.Err == strconv.ErrRange {
				// Accept the Inf value - this is valid JavaScript behavior
				lit.Value = value
			} else {
				// This suggests the lexer allowed an invalid float format (e.g., "1.2.3", "1e-e")
				msg := fmt.Sprintf("could not parse %q as float64: %v", rawLiteral, err)
				p.addError(p.curToken, msg)
				return nil
			}
		} else {
			lit.Value = value
		}
	} else {
		// Parse as integer first
		value, err := strconv.ParseInt(cleanedLiteral, base, 64)
		if err != nil {
			// Check if it's an overflow - for very large integers, fall back to float64 parsing
			// JavaScript numbers are IEEE 754 doubles, so this is the correct behavior
			if numErr, ok := err.(*strconv.NumError); ok && numErr.Err == strconv.ErrRange {
				// Try parsing as float64 instead
				floatVal, floatErr := strconv.ParseFloat(cleanedLiteral, 64)
				if floatErr != nil {
					msg := fmt.Sprintf("could not parse %q as number: %v", rawLiteral, floatErr)
					p.addError(p.curToken, msg)
					return nil
				}
				lit.Value = floatVal
			} else {
				// This suggests the lexer allowed invalid digits for the base or invalid format
				msg := fmt.Sprintf("could not parse %q as int (base %d): %v", rawLiteral, base, err)
				p.addError(p.curToken, msg)
				return nil
			}
		} else {
			// Store as float64 in the AST for simplicity/consistency
			lit.Value = float64(value)
		}
	}

	return lit
}

func (p *Parser) parseBigIntLiteral() Expression {
	lit := &BigIntLiteral{Token: p.curToken}

	rawLiteral := p.curToken.Literal

	// Remove the 'n' suffix to get the numeric part
	if !strings.HasSuffix(rawLiteral, "n") {
		msg := fmt.Sprintf("BigInt literal %q must end with 'n'", rawLiteral)
		p.addError(p.curToken, msg)
		return nil
	}

	// Extract numeric part (without 'n')
	numericPart := rawLiteral[:len(rawLiteral)-1]

	// Remove separators and validate format
	cleanedLiteral := strings.ReplaceAll(numericPart, "_", "")

	// BigInt can't be float, so we don't allow dots or scientific notation
	// We need to be careful not to reject valid hexadecimal digits like 'e' and 'f'
	if strings.Contains(cleanedLiteral, ".") {
		msg := fmt.Sprintf("BigInt literal %q cannot contain decimal point", rawLiteral)
		p.addError(p.curToken, msg)
		return nil
	}
	// Check for scientific notation patterns (e.g., "1e10", "1e+5", "1e-3")
	// But allow single 'e' or 'E' that might be part of hexadecimal digits
	if strings.Contains(cleanedLiteral, "e+") || strings.Contains(cleanedLiteral, "e-") ||
		strings.Contains(cleanedLiteral, "E+") || strings.Contains(cleanedLiteral, "E-") {
		msg := fmt.Sprintf("BigInt literal %q cannot contain scientific notation", rawLiteral)
		p.addError(p.curToken, msg)
		return nil
	}

	// Store the cleaned numeric part
	lit.Value = cleanedLiteral

	return lit
}

func (p *Parser) parseStringLiteral() Expression {
	return &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
}

// parseTemplateLiteral parses template literals with interpolations
// Expects current token to be TEMPLATE_START (`), processes tokens in sequence,
// ends with TEMPLATE_END (`). Always maintains string/expression alternation.
func (p *Parser) parseTemplateLiteral() Expression {
	debugPrint("parseTemplateLiteral: START - cur='%s' (%s), peek='%s' (%s)", p.curToken.Literal, p.curToken.Type, p.peekToken.Literal, p.peekToken.Type)
	lit := &TemplateLiteral{Token: p.curToken} // TEMPLATE_START token
	lit.Parts = []Node{}

	// Consume the opening backtick
	p.nextToken()
	debugPrint("parseTemplateLiteral: after consuming TEMPLATE_START - cur='%s' (%s), peek='%s' (%s)", p.curToken.Literal, p.curToken.Type, p.peekToken.Literal, p.peekToken.Type)

	// Always start with a string part (can be empty)
	expectingString := true

	for !p.curTokenIs(lexer.TEMPLATE_END) && !p.curTokenIs(lexer.EOF) {
		if p.curTokenIs(lexer.TEMPLATE_STRING) {
			if !expectingString {
				p.addError(p.curToken, "unexpected string in template literal")
				return nil
			}
			// String part of the template (include both cooked and raw values)
			stringPart := &TemplateStringPart{
				Value:             p.curToken.Literal,
				Raw:               p.curToken.RawLiteral,
				CookedIsUndefined: p.curToken.CookedIsUndefined,
			}
			lit.Parts = append(lit.Parts, stringPart)
			expectingString = false
			p.nextToken()
		} else if p.curTokenIs(lexer.TEMPLATE_INTERPOLATION) {
			// If we were expecting a string but got interpolation, add empty string
			if expectingString {
				emptyString := &TemplateStringPart{Value: "", Raw: "", CookedIsUndefined: false}
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
		emptyString := &TemplateStringPart{Value: "", Raw: "", CookedIsUndefined: false}
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

func (p *Parser) parseRegexLiteral() Expression {
	// Parse pattern and flags from the token literal /pattern/flags
	literal := p.curToken.Literal

	// Extract pattern and flags from the literal
	// The literal format is "/pattern/flags"
	if len(literal) < 2 || literal[0] != '/' {
		p.addError(p.curToken, fmt.Sprintf("Invalid regex literal format: %s", literal))
		return nil
	}

	// Find the closing slash
	lastSlash := -1
	for i := len(literal) - 1; i >= 1; i-- {
		if literal[i] == '/' {
			lastSlash = i
			break
		}
	}

	if lastSlash == -1 || lastSlash == 1 {
		p.addError(p.curToken, fmt.Sprintf("Invalid regex literal format: %s", literal))
		return nil
	}

	pattern := literal[1:lastSlash] // Extract pattern between slashes
	flags := literal[lastSlash+1:]  // Extract flags after last slash

	return &RegexLiteral{
		Token:   p.curToken,
		Pattern: pattern,
		Flags:   flags,
	}
}

func (p *Parser) parseThisExpression() Expression {
	return &ThisExpression{Token: p.curToken}
}

func (p *Parser) parseSuperExpression() Expression {
	return &SuperExpression{Token: p.curToken}
}

func (p *Parser) parseNewExpression() Expression {
	newToken := p.curToken // Save the 'new' token

	// Check if this is new.target
	if p.peekTokenIs(lexer.DOT) {
		p.nextToken() // Move to '.'
		if p.peekTokenIs(lexer.IDENT) && p.peekToken.Literal == "target" {
			p.nextToken() // Move to 'target'
			return &NewTargetExpression{Token: newToken}
		} else {
			// Error: new. followed by something other than 'target'
			p.addError(p.peekToken, "expected 'target' after 'new.'")
			return nil
		}
	}

	// Regular new expression
	ne := &NewExpression{Token: newToken}

	// Move to the next token (constructor identifier/expression)
	p.nextToken()

	// Parse the constructor expression (identifier, member expression, etc.)
	ne.Constructor = p.parseExpression(CALL)

	// Check for tagged template: new tag`template` -> new (tag`template`)
	// Tagged templates should be part of the constructor, not the arguments
	for p.peekTokenIs(lexer.TEMPLATE_START) {
		p.nextToken() // Move to template start (curToken is now TEMPLATE_START)
		template := p.parseTemplateLiteral()
		if template == nil {
			return nil
		}
		ne.Constructor = &TaggedTemplateExpression{
			Token:    p.curToken,
			Tag:      ne.Constructor,
			Template: template.(*TemplateLiteral),
		}
		// After parseTemplateLiteral, curToken is TEMPLATE_END
		// Don't advance - the checks below use peekToken
	}

	// Try to parse type arguments (e.g., new Container<string>)
	ne.TypeArguments = p.tryParseTypeArguments()

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

func (p *Parser) parseImportMetaExpression() Expression {
	importToken := p.curToken // Save the 'import' token

	// Check if this is import.meta or import.defer(...)
	if p.peekTokenIs(lexer.DOT) {
		p.nextToken() // Move to '.'
		if p.peekTokenIs(lexer.IDENT) && p.peekToken.Literal == "meta" {
			p.nextToken() // Move to 'meta'
			return &ImportMetaExpression{Token: importToken}
		} else if p.peekTokenIs(lexer.IDENT) && (p.peekToken.Literal == "defer" || p.peekToken.Literal == "source") {
			importPhase := p.peekToken.Literal
			p.nextToken() // Move to 'defer' or 'source'

			// Expect LPAREN after defer/source
			if !p.expectPeek(lexer.LPAREN) {
				p.addError(p.curToken, "expected '(' after 'import."+importPhase+"'")
				return nil
			}

			// Parse the argument list (should be exactly one expression)
			args := p.parseExpressionList(lexer.RPAREN)

			// import.defer() and import.source() require exactly one argument
			if len(args) == 0 {
				p.addError(importToken, "import."+importPhase+"() requires a module specifier argument")
				return nil
			}
			if len(args) > 1 {
				p.addError(importToken, "import."+importPhase+"() expects exactly one argument")
				return nil
			}

			// Both defer and source use the same AST node for now (deferred import)
			// They have similar semantics - loading modules in a deferred manner
			return &DeferredImportExpression{
				Token:  importToken,
				Source: args[0],
			}
		} else {
			// Error: import. followed by something other than 'meta', 'defer', or 'source'
			p.addError(p.peekToken, "expected 'meta', 'defer', or 'source' after 'import.'")
			return nil
		}
	}

	// Check if this is dynamic import: import(specifier) or import(specifier, options)
	if p.peekTokenIs(lexer.LPAREN) {
		p.nextToken() // Now curToken is LPAREN

		// Parse according to spec grammar:
		// import ( AssignmentExpression ,opt )
		// import ( AssignmentExpression , AssignmentExpression ,opt )

		// Check if we have an empty argument list: import()
		if p.peekTokenIs(lexer.RPAREN) {
			p.nextToken() // consume )
			p.addError(importToken, "import() requires a module specifier argument")
			return nil
		}

		// Parse first argument (specifier) as AssignmentExpression
		// Use ARG_SEPARATOR precedence to allow assignment but stop at commas
		p.nextToken() // advance to first argument
		specifier := p.parseExpression(ARG_SEPARATOR)
		if specifier == nil {
			p.addError(p.curToken, "import() requires a module specifier argument")
			return nil
		}

		var options Expression

		// Check for optional comma and second argument
		if p.peekTokenIs(lexer.COMMA) {
			p.nextToken() // consume comma

			// Check if there's a second argument (not just trailing comma)
			if !p.peekTokenIs(lexer.RPAREN) {
				p.nextToken()
				options = p.parseExpression(ARG_SEPARATOR)
				if options == nil {
					p.addError(p.curToken, "invalid import options expression")
					return nil
				}

				// Allow optional trailing comma after second argument
				if p.peekTokenIs(lexer.COMMA) {
					p.nextToken()
				}
			}
		}

		if !p.expectPeek(lexer.RPAREN) {
			return nil
		}

		return &DynamicImportExpression{
			Token:   importToken,
			Source:  specifier,
			Options: options,
		}
	}

	// If not import.meta, import.defer(), or import(), this is an error
	p.addError(p.curToken, "unexpected 'import' in expression context (expected import.meta, import.defer(), or import())")
	return nil
}

func (p *Parser) parseFunctionLiteral() Expression {
	lit := &FunctionLiteral{Token: p.curToken}

	// Check for generator syntax (function*)
	if p.peekTokenIs(lexer.ASTERISK) {
		p.nextToken() // Consume the '*'
		lit.IsGenerator = true
	}

	// Optional Function Name (can be IDENT or contextual keywords like yield, get, etc.)
	if p.peekTokenIs(lexer.IDENT) || p.peekTokenIs(lexer.YIELD) ||
		p.peekTokenIs(lexer.GET) || p.peekTokenIs(lexer.SET) ||
		p.peekTokenIs(lexer.THROW) || p.peekTokenIs(lexer.RETURN) ||
		p.peekTokenIs(lexer.LET) || p.peekTokenIs(lexer.AWAIT) {
		p.nextToken() // Consume name identifier/keyword
		// Create identifier from token (works for both IDENT and keywords)
		lit.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

	// Try to parse type parameters: function name<T, U>() or function<T, U>()
	lit.TypeParameters = p.tryParseTypeParameters()

	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	// --- MODIFIED: Use parseFunctionParameters ---
	lit.Parameters, lit.RestParameter, _ = p.parseFunctionParameters(false) // No parameter properties in function literals
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

	// Save and manage generator context when parsing body
	// Non-generator functions reset the context (even if nested in generators)
	// Generator functions increment the context
	savedGeneratorContext := p.inGenerator
	if lit.IsGenerator {
		p.inGenerator++
		if debugParser {
			fmt.Printf("[PARSER] Entering generator context, inGenerator=%d\n", p.inGenerator)
		}
	} else {
		// Non-generator function resets generator context (per ECMAScript spec)
		p.inGenerator = 0
		if debugParser && savedGeneratorContext > 0 {
			fmt.Printf("[PARSER] Resetting generator context for non-generator function (was %d)\n", savedGeneratorContext)
		}
	}

	// Save and manage async function context when parsing body
	// Async functions increment the context (nested async functions are allowed)
	// Non-async functions do NOT reset the context (they inherit parent context)
	// This allows top-level await to work while still disallowing await in non-async functions
	savedAsyncContext := p.inAsyncFunction
	if lit.IsAsync {
		p.inAsyncFunction++
		if debugParser {
			fmt.Printf("[PARSER] Entering async function context, inAsyncFunction=%d\n", p.inAsyncFunction)
		}
	}
	// Note: We do NOT reset inAsyncFunction for non-async functions
	// This allows top-level await to work, while await in non-async function bodies
	// will be caught by the type checker when type checking is enabled.

	lit.Body = p.parseBlockStatement() // Includes consuming RBRACE

	// Restore the saved generator context
	p.inGenerator = savedGeneratorContext
	if debugParser {
		fmt.Printf("[PARSER] Restored generator context to %d\n", p.inGenerator)
	}

	// Restore the saved async function context
	p.inAsyncFunction = savedAsyncContext
	if debugParser {
		fmt.Printf("[PARSER] Restored async function context to %d\n", p.inAsyncFunction)
	}

	// Transform function if it has destructuring parameters
	lit = p.transformFunctionWithDestructuring(lit)

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
	sig.Parameters, sig.RestParameter, _ = p.parseFunctionParameters(false) // No parameter properties in function type signatures
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
func (p *Parser) parseFunctionParameters(allowParameterProperties bool) ([]*Parameter, *RestParameter, error) {
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

	// Parse first regular parameter (could have access modifiers in constructor context)
	param := &Parameter{Token: p.curToken}

	// Check for access modifiers if we're in constructor parameter context
	if allowParameterProperties {
		for p.curTokenIs(lexer.PUBLIC) || p.curTokenIs(lexer.PRIVATE) || p.curTokenIs(lexer.PROTECTED) || p.curTokenIs(lexer.READONLY) {
			switch p.curToken.Type {
			case lexer.PUBLIC:
				param.IsPublic = true
			case lexer.PRIVATE:
				param.IsPrivate = true
			case lexer.PROTECTED:
				param.IsProtected = true
			case lexer.READONLY:
				param.IsReadonly = true
			}
			p.nextToken() // Consume access modifier and move to next token
		}
		// Update the parameter token to point to the actual parameter name
		param.Token = p.curToken
	}

	// Parse the parameter (could be 'this' parameter or destructuring pattern)
	// Allow YIELD as parameter name in non-generator functions (non-strict mode)
	isYieldParam := p.curTokenIs(lexer.YIELD) && p.inGenerator == 0
	// Allow AWAIT as parameter name in non-async functions
	isAwaitParam := p.curTokenIs(lexer.AWAIT) && p.inAsyncFunction == 0
	// Allow TYPE as parameter name (it's a contextual keyword in TypeScript, valid as identifier in JS)
	isTypeParam := p.curTokenIs(lexer.TYPE)
	if !p.curTokenIs(lexer.IDENT) && !p.curTokenIs(lexer.THIS) && !p.curTokenIs(lexer.LBRACKET) && !p.curTokenIs(lexer.LBRACE) && !isYieldParam && !isAwaitParam && !isTypeParam {
		msg := fmt.Sprintf("expected identifier, 'this', or destructuring pattern for parameter, got %s", p.curToken.Type)
		p.addError(p.curToken, msg)
		debugPrint("parseParameterList: Error - %s", msg)
		return nil, nil, fmt.Errorf("%s", msg)
	}

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
	} else if p.curTokenIs(lexer.LBRACKET) {
		// Array destructuring parameter
		param.IsDestructuring = true
		param.Pattern = p.parseArrayParameterPattern()
		if param.Pattern == nil {
			return nil, nil, fmt.Errorf("failed to parse array parameter pattern")
		}
	} else if p.curTokenIs(lexer.LBRACE) {
		// Object destructuring parameter
		param.IsDestructuring = true
		param.Pattern = p.parseObjectParameterPattern()
		if param.Pattern == nil {
			return nil, nil, fmt.Errorf("failed to parse object parameter pattern")
		}
	} else {
		// Regular identifier parameter
		param.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

	// Check for optional parameter (?)
	if p.peekTokenIs(lexer.QUESTION) {
		if param.IsThis {
			// Already handled above
		} else if param.IsDestructuring {
			p.addError(p.peekToken, "destructuring parameters cannot be optional")
			return nil, nil, fmt.Errorf("destructuring parameters cannot be optional")
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
		} else if param.IsDestructuring {
			// Allow destructuring parameters to have top-level default values
			// This is valid JavaScript/TypeScript syntax: function f({x} = {}) {}
			p.nextToken() // Consume '='
			p.nextToken() // Move to expression
			param.DefaultValue = p.parseExpression(COMMA)
			if param.DefaultValue == nil {
				return nil, nil, fmt.Errorf("expected expression after '=' in parameter default value")
			}
		} else {
			p.nextToken() // Consume '='
			p.nextToken() // Move to expression
			param.DefaultValue = p.parseExpression(COMMA)
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

		// Check for trailing comma (comma followed by closing paren)
		if p.peekTokenIs(lexer.RPAREN) {
			debugPrint("parseFunctionParameters: Found trailing comma, consuming closing paren")
			p.nextToken() // Consume ')'
			return parameters, restParam, nil
		}

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

		// Parse subsequent parameter (could have access modifiers in constructor context)
		param := &Parameter{Token: p.curToken}

		// Check for access modifiers if we're in constructor parameter context
		if allowParameterProperties {
			for p.curTokenIs(lexer.PUBLIC) || p.curTokenIs(lexer.PRIVATE) || p.curTokenIs(lexer.PROTECTED) || p.curTokenIs(lexer.READONLY) {
				switch p.curToken.Type {
				case lexer.PUBLIC:
					param.IsPublic = true
				case lexer.PRIVATE:
					param.IsPrivate = true
				case lexer.PROTECTED:
					param.IsProtected = true
				case lexer.READONLY:
					param.IsReadonly = true
				}
				p.nextToken() // Consume access modifier and move to next token
			}
			// Update the parameter token to point to the actual parameter name
			param.Token = p.curToken
		}

		// Allow YIELD as parameter name in non-generator functions (non-strict mode)
		isYieldParam := p.curTokenIs(lexer.YIELD) && p.inGenerator == 0
		// Allow AWAIT as parameter name in non-async functions
		isAwaitParam := p.curTokenIs(lexer.AWAIT) && p.inAsyncFunction == 0
		if !p.curTokenIs(lexer.IDENT) && !p.curTokenIs(lexer.LBRACKET) && !p.curTokenIs(lexer.LBRACE) && !isYieldParam && !isAwaitParam {
			msg := fmt.Sprintf("expected identifier or destructuring pattern for parameter after comma, got %s", p.curToken.Type)
			p.addError(p.curToken, msg)
			debugPrint("parseParameterList: Error - %s", msg)
			return nil, nil, fmt.Errorf("%s", msg)
		}

		if p.curTokenIs(lexer.LBRACKET) {
			// Array destructuring parameter
			param.IsDestructuring = true
			param.Pattern = p.parseArrayParameterPattern()
			if param.Pattern == nil {
				return nil, nil, fmt.Errorf("failed to parse array parameter pattern")
			}
		} else if p.curTokenIs(lexer.LBRACE) {
			// Object destructuring parameter
			param.IsDestructuring = true
			param.Pattern = p.parseObjectParameterPattern()
			if param.Pattern == nil {
				return nil, nil, fmt.Errorf("failed to parse object parameter pattern")
			}
		} else {
			// Regular identifier parameter
			param.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		}

		// Check for optional parameter (?)
		if p.peekTokenIs(lexer.QUESTION) {
			if param.IsDestructuring {
				p.addError(p.peekToken, "destructuring parameters cannot be optional")
				return nil, nil, fmt.Errorf("destructuring parameters cannot be optional")
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
			param.TypeAnnotation = nil
		}

		// Check for Default Value
		if p.peekTokenIs(lexer.ASSIGN) {
			if param.IsDestructuring {
				// Allow destructuring parameters to have top-level default values
				// This is valid JavaScript/TypeScript syntax: function f({x} = {}) {}
				p.nextToken() // Consume '='
				p.nextToken() // Move to expression
				param.DefaultValue = p.parseExpression(COMMA)
				if param.DefaultValue == nil {
					return nil, nil, fmt.Errorf("expected expression after '=' in parameter default value")
				}
			} else {
				p.nextToken() // Consume '='
				p.nextToken() // Move to expression
				param.DefaultValue = p.parseExpression(COMMA)
				if param.DefaultValue == nil {
					p.addError(p.curToken, "expected expression after '=' in parameter default value")
					return nil, nil, fmt.Errorf("expected expression after '=' in parameter default value")
				}
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

	// Expect identifier or destructuring pattern after '...'
	if !p.peekTokenIs(lexer.IDENT) && !p.peekTokenIs(lexer.LBRACKET) && !p.peekTokenIs(lexer.LBRACE) {
		p.addError(p.peekToken, "expected identifier or destructuring pattern after '...' in rest parameter")
		return nil
	}

	p.nextToken() // Move to the identifier or pattern

	// Check if it's a destructuring pattern
	if p.curTokenIs(lexer.LBRACKET) {
		// Array destructuring: ...[x, y]
		restParam.Pattern = p.parseArrayParameterPattern()
		if restParam.Pattern == nil {
			return nil
		}
	} else if p.curTokenIs(lexer.LBRACE) {
		// Object destructuring: ...{x, y}
		restParam.Pattern = p.parseObjectParameterPattern()
		if restParam.Pattern == nil {
			return nil
		}
	} else {
		// Regular identifier
		restParam.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

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

// --- NEW: Parameter Pattern Parsing Functions ---

// parseArrayParameterPattern parses array destructuring in function parameters
// Examples: [a, b], [a = 1, b], [first, ...rest]
func (p *Parser) parseArrayParameterPattern() *ArrayParameterPattern {
	pattern := &ArrayParameterPattern{
		Token:    p.curToken,                // The '[' token
		Elements: []*DestructuringElement{}, // Initialize to empty slice to prevent nil pointer
	}

	// Parse elements using existing destructuring logic but adapted for parameters
	elements := []*DestructuringElement{}

	// Handle empty array pattern: []
	if p.peekTokenIs(lexer.RBRACKET) {
		p.nextToken() // Consume ']'
		pattern.Elements = elements
		return pattern
	}

	// Parse elements, handling elisions
	for {
		p.nextToken() // Move to next position

		// Check for end of array
		if p.curTokenIs(lexer.RBRACKET) {
			break
		}

		// Check for elision (comma with no element before it)
		if p.curTokenIs(lexer.COMMA) {
			// Elision: [, x] or [x, , y]
			elements = append(elements, &DestructuringElement{Target: nil})
			// Check if this is the end: [,] or [x,]
			if p.peekTokenIs(lexer.RBRACKET) {
				p.nextToken() // Consume ]
				break
			}
			// More elements follow, continue to parse them
			continue
		}

		// Parse actual element
		element := p.parseParameterDestructuringElement()
		if element == nil {
			p.addError(p.curToken, "failed to parse array parameter element")
			return nil
		}
		elements = append(elements, element)

		// If this is a rest element, it must be the last one
		if element.IsRest {
			if !p.expectPeek(lexer.RBRACKET) {
				p.addError(p.peekToken, "rest element must be last element in array parameter")
				return nil
			}
			break
		}

		// Check for more elements
		if !p.peekTokenIs(lexer.COMMA) {
			// No more elements, expect ]
			if !p.expectPeek(lexer.RBRACKET) {
				p.addError(p.peekToken, "expected ']' after array parameter elements")
				return nil
			}
			break
		}
		// Consume comma and continue
		p.nextToken()
	}

	pattern.Elements = elements
	return pattern
}

// Examples: {x, y}, {x = 1, y}, {x: localX, ...rest}
func (p *Parser) parseObjectParameterPattern() *ObjectParameterPattern {
	pattern := &ObjectParameterPattern{Token: p.curToken} // The '{' token

	properties := []*DestructuringProperty{}
	var restProperty *DestructuringElement

	// Handle empty object pattern: {}
	if p.peekTokenIs(lexer.RBRACE) {
		p.nextToken() // Consume '}'
		pattern.Properties = properties
		return pattern
	}

	p.nextToken() // Move to first property

	// Parse first property/rest
	if p.curTokenIs(lexer.SPREAD) {
		// Rest property
		restProperty = p.parseParameterDestructuringElement()
		if restProperty == nil {
			return nil
		}
		if !restProperty.IsRest {
			p.addError(p.curToken, "expected rest element after '...'")
			return nil
		}
	} else {
		// Regular property
		prop := p.parseParameterDestructuringProperty()
		if prop == nil {
			return nil
		}
		properties = append(properties, prop)
	}

	// Parse subsequent properties
	for p.peekTokenIs(lexer.COMMA) && restProperty == nil {
		p.nextToken() // Consume ','

		// Check for trailing comma
		if p.peekTokenIs(lexer.RBRACE) {
			break
		}

		p.nextToken() // Move to next property

		if p.curTokenIs(lexer.SPREAD) {
			// Rest property (must be last)
			restProperty = p.parseParameterDestructuringElement()
			if restProperty == nil {
				return nil
			}
			if !restProperty.IsRest {
				p.addError(p.curToken, "expected rest element after '...'")
				return nil
			}
			break // Rest must be last
		} else {
			// Regular property
			prop := p.parseParameterDestructuringProperty()
			if prop == nil {
				return nil
			}
			properties = append(properties, prop)
		}
	}

	if !p.expectPeek(lexer.RBRACE) {
		p.addError(p.peekToken, "expected '}' after object parameter properties")
		return nil
	}

	pattern.Properties = properties
	pattern.RestProperty = restProperty
	return pattern
}

// parseParameterDestructuringElement parses a destructuring element in parameter context
// Handles: identifier, ...rest, identifier = default
func (p *Parser) parseParameterDestructuringElement() *DestructuringElement {
	element := &DestructuringElement{}

	// Check for rest element
	if p.curTokenIs(lexer.SPREAD) {
		element.IsRest = true
		// Rest element can have: ...identifier, ...[array], or ...{object}
		if !p.peekTokenIs(lexer.IDENT) && !p.peekTokenIs(lexer.LBRACKET) && !p.peekTokenIs(lexer.LBRACE) {
			p.addError(p.peekToken, "expected identifier or pattern after '...' in parameter rest element")
			return nil
		}
		p.nextToken() // Move to the target
	}

	// Parse target (support nested patterns in parameter context)
	if p.curTokenIs(lexer.IDENT) {
		element.Target = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	} else if p.curTokenIs(lexer.LBRACKET) {
		// Nested array destructuring: function f([a, [b, c]]) or function f([...[x, y]])
		// IMPORTANT: Must use parseArrayParameterPattern, not parseArrayLiteral
		// because nested patterns can have defaults: [[x] = [99]]
		element.Target = p.parseArrayParameterPattern()
		if element.Target == nil {
			return nil
		}
	} else if p.curTokenIs(lexer.LBRACE) {
		// Nested object destructuring: function f({user: {name, age}}) or function f([...{a, b}])
		// IMPORTANT: Must use parseObjectParameterPattern, not parseObjectLiteral
		// because nested patterns can have defaults: [{x} = {}]
		element.Target = p.parseObjectParameterPattern()
		if element.Target == nil {
			return nil
		}
	} else {
		p.addError(p.curToken, "parameter destructuring target must be an identifier or nested pattern")
		return nil
	}

	// Check for default value (not allowed for rest elements)
	if !element.IsRest && p.peekTokenIs(lexer.ASSIGN) {
		p.nextToken() // Consume '='
		p.nextToken() // Move to default expression
		// Parse with COMMA precedence to exclude comma operator from default expressions
		element.Default = p.parseExpression(COMMA)
		if element.Default == nil {
			p.addError(p.curToken, "expected expression after '=' in parameter default")
			return nil
		}
	}

	return element
}

// parseParameterDestructuringProperty parses a property in object parameter destructuring
// Handles: key, key: target, key = default, key: target = default, [computed]: target
func (p *Parser) parseParameterDestructuringProperty() *DestructuringProperty {
	prop := &DestructuringProperty{}

	// Parse key (identifier or computed property)
	if p.curTokenIs(lexer.LBRACKET) {
		// Computed property: [expression]: target
		p.nextToken() // Move past '['
		// Parse the computed expression
		computedExpr := p.parseExpression(LOWEST)
		if computedExpr == nil {
			p.addError(p.curToken, "expected expression in computed property key")
			return nil
		}
		if !p.expectPeek(lexer.RBRACKET) {
			p.addError(p.peekToken, "expected ']' after computed property key")
			return nil
		}
		// Wrap in ComputedPropertyName
		prop.Key = &ComputedPropertyName{
			Expr: computedExpr,
		}
		// Computed properties must have explicit target
		if !p.expectPeek(lexer.COLON) {
			p.addError(p.peekToken, "computed property in destructuring must have explicit target")
			return nil
		}
		p.nextToken() // Move to target
		// Parse target
		if p.curTokenIs(lexer.IDENT) {
			prop.Target = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		} else if p.curTokenIs(lexer.LBRACKET) {
			prop.Target = p.parseArrayLiteral()
			if prop.Target == nil {
				return nil
			}
		} else if p.curTokenIs(lexer.LBRACE) {
			prop.Target = p.parseObjectLiteral()
			if prop.Target == nil {
				return nil
			}
		} else {
			p.addError(p.curToken, "computed property target must be an identifier or nested pattern")
			return nil
		}
	} else if p.curTokenIs(lexer.IDENT) || p.isKeywordThatCanBeIdentifier(p.curToken.Type) {
		// Regular identifier key (includes contextual keywords like FROM, OF, TYPE, etc.)
		prop.Key = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	} else if p.curTokenIs(lexer.NUMBER) {
		// Numeric property key (e.g., {0: v, 1: w})
		// Convert to NumberLiteral for the key
		numVal := 0.0
		fmt.Sscanf(p.curToken.Literal, "%f", &numVal)
		prop.Key = &NumberLiteral{Token: p.curToken, Value: numVal}
	} else {
		p.addError(p.curToken, "object parameter property key must be an identifier, number, or computed property")
		return nil
	}

	// For regular identifiers and numbers, check for explicit target (key: target)
	_, isIdent := prop.Key.(*Identifier)
	_, isNumber := prop.Key.(*NumberLiteral)
	if (isIdent || isNumber) && p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ':'
		p.nextToken() // Move to target

		// Parse target (support nested patterns)
		if p.curTokenIs(lexer.IDENT) {
			prop.Target = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		} else if p.curTokenIs(lexer.LBRACKET) {
			// Nested array destructuring: {prop: [a, b]}
			prop.Target = p.parseArrayLiteral()
			if prop.Target == nil {
				return nil
			}
		} else if p.curTokenIs(lexer.LBRACE) {
			// Nested object destructuring: {prop: {x, y}}
			prop.Target = p.parseObjectLiteral()
			if prop.Target == nil {
				return nil
			}
		} else {
			p.addError(p.curToken, "object parameter property target must be an identifier or nested pattern")
			return nil
		}
	} else {
		// Shorthand: use key as target
		prop.Target = prop.Key
	}

	// Check for default value
	if p.peekTokenIs(lexer.ASSIGN) {
		p.nextToken() // Consume '='
		p.nextToken() // Move to default expression
		// Parse with COMMA precedence to exclude comma operator from default expressions
		prop.Default = p.parseExpression(COMMA)
		if prop.Default == nil {
			p.addError(p.curToken, "expected expression after '=' in parameter default")
			return nil
		}
	}

	return prop
}

// --- END NEW: Parameter Pattern Parsing Functions ---

// --- NEW: Parameter Transformation Functions ---

// transformFunctionWithDestructuring transforms a function with destructuring parameters
// into a function with regular parameters and destructuring assignments in the body
func (p *Parser) transformFunctionWithDestructuring(fn *FunctionLiteral) *FunctionLiteral {
	if fn == nil || fn.Body == nil {
		return fn
	}

	// Check if any parameters use destructuring
	hasDestructuring := false
	for _, param := range fn.Parameters {
		if param.IsDestructuring {
			hasDestructuring = true
			break
		}
	}

	if !hasDestructuring {
		return fn // No transformation needed
	}

	// Create new parameters and body statements
	newParams := []*Parameter{}
	newStatements := []Statement{}
	paramIndex := 0

	for _, param := range fn.Parameters {
		if param.IsDestructuring {
			// Create a new regular parameter for this destructuring parameter
			newParamName := fmt.Sprintf("__destructured_param_%d", paramIndex)
			newParam := &Parameter{
				Token:           param.Token,
				Name:            &Identifier{Token: param.Token, Value: newParamName},
				TypeAnnotation:  param.TypeAnnotation,
				ComputedType:    param.ComputedType,
				Optional:        false,              // Destructuring params can't be optional
				DefaultValue:    param.DefaultValue, // Preserve default value for destructuring parameters
				IsThis:          false,
				IsDestructuring: false,
			}
			newParams = append(newParams, newParam)

			// Create destructuring declaration statement
			if arrayPattern, ok := param.Pattern.(*ArrayParameterPattern); ok {
				// Convert array parameter pattern to array destructuring declaration
				declaration := &ArrayDestructuringDeclaration{
					Token:          arrayPattern.Token,
					IsConst:        false, // Use let for function parameters
					Elements:       arrayPattern.Elements,
					TypeAnnotation: param.TypeAnnotation,
					Value: &Identifier{
						Token: lexer.Token{
							Type:     lexer.IDENT,
							Literal:  newParamName,
							Line:     param.Token.Line,
							Column:   param.Token.Column,
							StartPos: param.Token.StartPos,
						},
						Value: newParamName,
					},
				}
				newStatements = append(newStatements, declaration)
			} else if objectPattern, ok := param.Pattern.(*ObjectParameterPattern); ok {
				// Convert object parameter pattern to object destructuring declaration
				declaration := &ObjectDestructuringDeclaration{
					Token:          objectPattern.Token,
					IsConst:        false, // Use let for function parameters
					Properties:     objectPattern.Properties,
					RestProperty:   objectPattern.RestProperty,
					TypeAnnotation: param.TypeAnnotation,
					Value: &Identifier{
						Token: lexer.Token{
							Type:     lexer.IDENT,
							Literal:  newParamName,
							Line:     param.Token.Line,
							Column:   param.Token.Column,
							StartPos: param.Token.StartPos,
						},
						Value: newParamName,
					},
				}
				newStatements = append(newStatements, declaration)
			}
			paramIndex++
		} else {
			// Regular parameter - keep as is
			newParams = append(newParams, param)
		}
	}

	// Combine new destructuring statements with original body
	newStatements = append(newStatements, fn.Body.Statements...)

	// Create new function with transformed parameters and body
	newFn := &FunctionLiteral{
		BaseExpression:       fn.BaseExpression,
		Token:                fn.Token,
		Name:                 fn.Name,
		IsGenerator:          fn.IsGenerator, // Preserve generator flag
		IsAsync:              fn.IsAsync,     // Preserve async flag
		Parameters:           newParams,
		RestParameter:        fn.RestParameter, // Rest parameter stays the same
		ReturnTypeAnnotation: fn.ReturnTypeAnnotation,
		Body: &BlockStatement{
			Token:               fn.Body.Token,
			Statements:          newStatements,
			HoistedDeclarations: fn.Body.HoistedDeclarations,
		},
	}

	return newFn
}

// transformArrowFunctionWithDestructuring transforms an arrow function with destructuring parameters
func (p *Parser) transformArrowFunctionWithDestructuring(fn *ArrowFunctionLiteral) *ArrowFunctionLiteral {
	if fn == nil {
		return fn
	}

	// Check if any parameters use destructuring
	hasDestructuring := false
	for _, param := range fn.Parameters {
		if param.IsDestructuring {
			hasDestructuring = true
			break
		}
	}

	if !hasDestructuring {
		return fn // No transformation needed
	}

	// Create new parameters and body statements
	newParams := []*Parameter{}
	newStatements := []Statement{}
	paramIndex := 0

	for _, param := range fn.Parameters {
		if param.IsDestructuring {
			// Create a new regular parameter for this destructuring parameter
			newParamName := fmt.Sprintf("__destructured_param_%d", paramIndex)
			newParam := &Parameter{
				Token:           param.Token,
				Name:            &Identifier{Token: param.Token, Value: newParamName},
				TypeAnnotation:  param.TypeAnnotation,
				ComputedType:    param.ComputedType,
				Optional:        false,
				DefaultValue:    param.DefaultValue, // Preserve default value for destructuring parameters
				IsThis:          false,
				IsDestructuring: false,
			}
			newParams = append(newParams, newParam)

			// Create destructuring declaration statement
			if arrayPattern, ok := param.Pattern.(*ArrayParameterPattern); ok {
				// Create a 'let' token for the declaration
				letToken := lexer.Token{
					Type:     lexer.LET,
					Literal:  "let",
					Line:     arrayPattern.Token.Line,
					Column:   arrayPattern.Token.Column,
					StartPos: arrayPattern.Token.StartPos,
				}
				declaration := &ArrayDestructuringDeclaration{
					Token:          letToken,
					IsConst:        false, // Use let for function parameters
					Elements:       arrayPattern.Elements,
					TypeAnnotation: param.TypeAnnotation,
					Value: &Identifier{
						Token: lexer.Token{
							Type:     lexer.IDENT,
							Literal:  newParamName,
							Line:     param.Token.Line,
							Column:   param.Token.Column,
							StartPos: param.Token.StartPos,
						},
						Value: newParamName,
					},
				}
				newStatements = append(newStatements, declaration)
			} else if objectPattern, ok := param.Pattern.(*ObjectParameterPattern); ok {
				// Create a 'let' token for the declaration
				letToken := lexer.Token{
					Type:     lexer.LET,
					Literal:  "let",
					Line:     objectPattern.Token.Line,
					Column:   objectPattern.Token.Column,
					StartPos: objectPattern.Token.StartPos,
				}
				declaration := &ObjectDestructuringDeclaration{
					Token:          letToken,
					IsConst:        false, // Use let for function parameters
					Properties:     objectPattern.Properties,
					RestProperty:   objectPattern.RestProperty,
					TypeAnnotation: param.TypeAnnotation,
					Value: &Identifier{
						Token: lexer.Token{
							Type:     lexer.IDENT,
							Literal:  newParamName,
							Line:     param.Token.Line,
							Column:   param.Token.Column,
							StartPos: param.Token.StartPos,
						},
						Value: newParamName,
					},
				}
				newStatements = append(newStatements, declaration)
			}
			paramIndex++
		} else {
			// Regular parameter - keep as is
			newParams = append(newParams, param)
		}
	}

	// Handle arrow function body transformation
	var newBody Node
	if blockStmt, ok := fn.Body.(*BlockStatement); ok {
		// Block statement body - combine new destructuring statements with original body
		newStatements = append(newStatements, blockStmt.Statements...)
		newBody = &BlockStatement{
			Token:               blockStmt.Token,
			Statements:          newStatements,
			HoistedDeclarations: blockStmt.HoistedDeclarations,
		}
	} else {
		// Expression body - convert to block statement with destructuring assignments + return
		if len(newStatements) > 0 {
			returnStmt := &ReturnStatement{
				Token:       fn.Token, // Use arrow function token
				ReturnValue: fn.Body.(Expression),
			}
			newStatements = append(newStatements, returnStmt)
			newBody = &BlockStatement{
				Token:      fn.Token,
				Statements: newStatements,
			}
		} else {
			// No destructuring, keep expression body
			newBody = fn.Body
		}
	}

	// Create new arrow function with transformed parameters and body
	newFn := &ArrowFunctionLiteral{
		BaseExpression:       fn.BaseExpression,
		Token:                fn.Token,
		Parameters:           newParams,
		RestParameter:        fn.RestParameter,
		ReturnTypeAnnotation: fn.ReturnTypeAnnotation,
		Body:                 newBody,
	}

	return newFn
}

// transformShorthandMethodWithDestructuring transforms a shorthand method with destructuring parameters
func (p *Parser) transformShorthandMethodWithDestructuring(method *ShorthandMethod) *ShorthandMethod {
	if method == nil || method.Body == nil {
		return method
	}

	// Check if any parameters use destructuring
	hasDestructuring := false
	for _, param := range method.Parameters {
		if param.IsDestructuring {
			hasDestructuring = true
			break
		}
	}

	if !hasDestructuring {
		return method // No transformation needed
	}

	// Create new parameters and body statements
	newParams := []*Parameter{}
	newStatements := []Statement{}
	paramIndex := 0

	for _, param := range method.Parameters {
		if param.IsDestructuring {
			// Create a new regular parameter for this destructuring parameter
			newParamName := fmt.Sprintf("__destructured_param_%d", paramIndex)
			newParam := &Parameter{
				Token:           param.Token,
				Name:            &Identifier{Token: param.Token, Value: newParamName},
				TypeAnnotation:  param.TypeAnnotation,
				ComputedType:    param.ComputedType,
				Optional:        false,
				DefaultValue:    param.DefaultValue, // Preserve default value for destructuring parameters
				IsThis:          false,
				IsDestructuring: false,
			}
			newParams = append(newParams, newParam)

			// Create destructuring declaration statement
			if arrayPattern, ok := param.Pattern.(*ArrayParameterPattern); ok {
				// Create a 'let' token for the declaration
				letToken := lexer.Token{
					Type:     lexer.LET,
					Literal:  "let",
					Line:     arrayPattern.Token.Line,
					Column:   arrayPattern.Token.Column,
					StartPos: arrayPattern.Token.StartPos,
				}
				declaration := &ArrayDestructuringDeclaration{
					Token:          letToken,
					IsConst:        false, // Use let for function parameters
					Elements:       arrayPattern.Elements,
					TypeAnnotation: param.TypeAnnotation,
					Value: &Identifier{
						Token: lexer.Token{
							Type:     lexer.IDENT,
							Literal:  newParamName,
							Line:     param.Token.Line,
							Column:   param.Token.Column,
							StartPos: param.Token.StartPos,
						},
						Value: newParamName,
					},
				}
				newStatements = append(newStatements, declaration)
			} else if objectPattern, ok := param.Pattern.(*ObjectParameterPattern); ok {
				// Create a 'let' token for the declaration
				letToken := lexer.Token{
					Type:     lexer.LET,
					Literal:  "let",
					Line:     objectPattern.Token.Line,
					Column:   objectPattern.Token.Column,
					StartPos: objectPattern.Token.StartPos,
				}
				declaration := &ObjectDestructuringDeclaration{
					Token:          letToken,
					IsConst:        false, // Use let for function parameters
					Properties:     objectPattern.Properties,
					RestProperty:   objectPattern.RestProperty,
					TypeAnnotation: param.TypeAnnotation,
					Value: &Identifier{
						Token: lexer.Token{
							Type:     lexer.IDENT,
							Literal:  newParamName,
							Line:     param.Token.Line,
							Column:   param.Token.Column,
							StartPos: param.Token.StartPos,
						},
						Value: newParamName,
					},
				}
				newStatements = append(newStatements, declaration)
			}
			paramIndex++
		} else {
			// Regular parameter - keep as is
			newParams = append(newParams, param)
		}
	}

	// Combine new destructuring statements with original body
	newStatements = append(newStatements, method.Body.Statements...)

	// Create new method with transformed parameters and body
	newMethod := &ShorthandMethod{
		BaseExpression:       method.BaseExpression,
		Token:                method.Token,
		Name:                 method.Name,
		Parameters:           newParams,
		RestParameter:        method.RestParameter,
		ReturnTypeAnnotation: method.ReturnTypeAnnotation,
		Body: &BlockStatement{
			Token:               method.Body.Token,
			Statements:          newStatements,
			HoistedDeclarations: method.Body.HoistedDeclarations,
		},
	}

	return newMethod
}

// --- END NEW: Parameter Transformation Functions ---

func (p *Parser) parseSpreadElement() Expression {
	spreadElement := &SpreadElement{Token: p.curToken} // The '...' token

	// Parse the expression after '...'
	p.nextToken() // Move to the expression
	// Per ECMAScript spec: SpreadElement evaluates AssignmentExpression
	// Use ARG_SEPARATOR precedence to allow assignment but stop at commas
	// This allows: [...x = [1, 2]] and test(...x = [1, 2])
	spreadElement.Argument = p.parseExpression(ARG_SEPARATOR)
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
			if exprStmt, isExprStmt := stmt.(*ExpressionStatement); isExprStmt && exprStmt.Expression != nil {
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

// peekTokenIs2 checks if the token after peekToken matches the given type
// This requires looking ahead 2 tokens from current position
func (p *Parser) peekTokenIs2(t lexer.TokenType) bool {
	// Save current state
	curToken := p.curToken
	peekToken := p.peekToken

	// Advance once to look at token after peek
	p.nextToken()
	result := p.peekTokenIs(t)

	// Restore state
	p.curToken = curToken
	p.peekToken = peekToken

	return result
}

// lookAhead returns the token at position 'pos' ahead of peekToken
// pos=0 returns peekToken, pos=1 returns the token after peekToken, etc.
// Uses lexer state save/restore to avoid corrupting parser state
func (p *Parser) lookAhead(pos int) lexer.Token {
	if pos == 0 {
		return p.peekToken
	}

	// Save complete lexer state
	savedState := p.l.SaveState()
	savedCur := p.curToken
	savedPeek := p.peekToken

	// Advance pos+1 times to get to the desired position
	// (pos=1 means one token after peekToken)
	var token lexer.Token
	for i := 0; i <= pos; i++ {
		token = p.l.NextToken()
	}

	// Restore complete lexer state
	p.l.RestoreState(savedState)
	p.curToken = savedCur
	p.peekToken = savedPeek

	return token
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

// isKeywordThatCanBeIdentifier checks if a token type is a keyword that can be used as an identifier
func (p *Parser) isKeywordThatCanBeIdentifier(tokenType lexer.TokenType) bool {
	// In JavaScript/TypeScript, all keywords can be used as property names in object literals
	// This matches the list in parsePropertyName()
	switch tokenType {
	case lexer.DELETE, lexer.GET, lexer.SET, lexer.IF, lexer.ELSE, lexer.FOR, lexer.WHILE, lexer.FUNCTION,
		lexer.RETURN, lexer.THROW, lexer.LET, lexer.CONST, lexer.TRUE, lexer.FALSE, lexer.NULL,
		lexer.UNDEFINED, lexer.THIS, lexer.NEW, lexer.TYPEOF, lexer.VOID, lexer.AS, lexer.SATISFIES,
		lexer.IN, lexer.INSTANCEOF, lexer.DO, lexer.ENUM, lexer.FROM, lexer.CATCH, lexer.FINALLY,
		lexer.TRY, lexer.SWITCH, lexer.CASE, lexer.DEFAULT, lexer.BREAK, lexer.CONTINUE, lexer.CLASS,
		lexer.STATIC, lexer.READONLY, lexer.PUBLIC, lexer.PRIVATE, lexer.PROTECTED, lexer.ABSTRACT,
		lexer.OVERRIDE, lexer.IMPORT, lexer.EXPORT, lexer.YIELD, lexer.AWAIT, lexer.VAR, lexer.TYPE, lexer.KEYOF,
		lexer.INFER, lexer.IS, lexer.OF, lexer.INTERFACE, lexer.EXTENDS, lexer.IMPLEMENTS, lexer.SUPER, lexer.WITH:
		return true
	default:
		return false
	}
}

// canStartExpression checks if a token can be the start of an expression
// This is used to determine if 'await' should be parsed as an AwaitExpression or as an Identifier
func (p *Parser) canStartExpression(t lexer.Token) bool {
	switch t.Type {
	case lexer.IDENT, lexer.NUMBER, lexer.BIGINT, lexer.STRING, lexer.REGEX_LITERAL,
		lexer.TEMPLATE_START, lexer.TRUE, lexer.FALSE, lexer.NULL, lexer.UNDEFINED,
		lexer.THIS, lexer.NEW, lexer.IMPORT, lexer.FUNCTION, lexer.ASYNC, lexer.CLASS,
		lexer.BANG, lexer.MINUS, lexer.PLUS, lexer.BITWISE_NOT, lexer.TYPEOF, lexer.VOID,
		lexer.DELETE, lexer.YIELD, lexer.AWAIT, lexer.INC, lexer.DEC,
		lexer.LPAREN, lexer.LT, lexer.IF, lexer.LBRACKET, lexer.LBRACE, lexer.SPREAD,
		lexer.SUPER, lexer.GET, lexer.SET, lexer.LET, lexer.THROW, lexer.RETURN:
		return true
	default:
		return false
	}
}

// expectPeekIdentifierOrKeyword accepts IDENT or contextual keywords that can be used as identifiers.
// Returns true if peek is IDENT, YIELD, GET, THROW, or RETURN and advances.
func (p *Parser) expectPeekIdentifierOrKeyword() bool {
	if p.peekTokenIs(lexer.IDENT) || p.peekTokenIs(lexer.YIELD) ||
		p.peekTokenIs(lexer.GET) || p.peekTokenIs(lexer.THROW) || p.peekTokenIs(lexer.RETURN) {
		p.nextToken()
		return true
	} else {
		msg := fmt.Sprintf("expected identifier or destructuring pattern after '%s', got %s", p.curToken.Literal, p.peekToken.Type)
		p.addError(p.peekToken, msg)
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

// parseYieldExpression handles yield expressions in generator functions or yield as identifier
func (p *Parser) parseYieldExpression() Expression {
	yieldToken := p.curToken

	// If we're inside a generator, always treat as yield expression
	if p.inGenerator > 0 {
		if debugParser {
			fmt.Printf("[PARSER] Parsing yield expression (inGenerator=%d)\n", p.inGenerator)
		}

		expression := &YieldExpression{
			Token: yieldToken, // The 'yield' token
		}

		// Check if next token is yield* delegation
		if p.peekTokenIs(lexer.ASTERISK) {
			p.nextToken() // Move to '*'
			expression.Delegate = true
		}

		// yield can have an optional value
		// Check if there's an expression following yield (or yield*)
		// Don't try to parse if next token is closing punctuation or comma
		if !p.peekTokenIs(lexer.SEMICOLON) && !p.peekTokenIs(lexer.RBRACE) &&
			!p.peekTokenIs(lexer.RBRACKET) && !p.peekTokenIs(lexer.RPAREN) &&
			!p.peekTokenIs(lexer.COMMA) && !p.peekTokenIs(lexer.EOF) &&
			!p.peekTokenIs(lexer.COLON) { // for switch cases and object properties
			// Advance to the value expression
			p.nextToken()
			// Parse the value to yield with ASSIGNMENT precedence (stops at commas)
			// This prevents yield from consuming comma operators in contexts like object literals
			expression.Value = p.parseExpression(ASSIGNMENT)
			if expression.Value == nil {
				// If parsing failed, treat as yield with no value
				expression.Value = nil
			}
		}

		return expression
	}

	// Not in generator context - use heuristic to determine if yield is identifier or error
	// Heuristic: if followed by punctuation/operators that suggest end of statement or usage as value,
	// treat as identifier. Otherwise, it's likely an error (yield expression outside generator).
	// Common identifier contexts: yield), yield,, yield;, yield}, yield], yield.prop, yield + x
	if p.peekTokenIs(lexer.RPAREN) || p.peekTokenIs(lexer.COMMA) ||
		p.peekTokenIs(lexer.SEMICOLON) || p.peekTokenIs(lexer.RBRACE) ||
		p.peekTokenIs(lexer.RBRACKET) || p.peekTokenIs(lexer.EOF) ||
		p.peekTokenIs(lexer.COLON) || // for object properties
		p.peekTokenIs(lexer.DOT) || // for member access
		p.peekTokenIs(lexer.ASSIGN) || // for assignments
		p.peekTokenIs(lexer.PLUS) || p.peekTokenIs(lexer.MINUS) ||
		p.peekTokenIs(lexer.ASTERISK) || p.peekTokenIs(lexer.SLASH) ||
		p.peekTokenIs(lexer.GT) || p.peekTokenIs(lexer.LT) ||
		p.peekTokenIs(lexer.EQ) || p.peekTokenIs(lexer.NOT_EQ) {
		// Treat as identifier in non-generator context
		return &Identifier{Token: yieldToken, Value: yieldToken.Literal}
	}

	// Otherwise, parse as yield expression (will be caught as error by type checker)
	expression := &YieldExpression{
		Token: yieldToken,
	}

	if p.peekTokenIs(lexer.ASTERISK) {
		p.nextToken() // Move to '*'
		expression.Delegate = true
	}

	if !p.peekTokenIs(lexer.SEMICOLON) && !p.peekTokenIs(lexer.RBRACE) &&
		!p.peekTokenIs(lexer.RBRACKET) && !p.peekTokenIs(lexer.RPAREN) &&
		!p.peekTokenIs(lexer.COMMA) && !p.peekTokenIs(lexer.EOF) &&
		!p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Advance to value expression
		expression.Value = p.parseExpression(LOWEST)
	}

	return expression
}

// parseAsyncExpression handles async function expressions and async arrow functions
// async function() { ... }
// async () => { ... }
// async x => x
func (p *Parser) parseAsyncExpression() Expression {
	asyncToken := p.curToken // Save the 'async' token

	p.nextToken() // Move past 'async'

	// Check if this is an async function expression
	if p.curTokenIs(lexer.FUNCTION) {
		funcExpr := p.parseFunctionLiteral()
		if funcLit, ok := funcExpr.(*FunctionLiteral); ok {
			funcLit.IsAsync = true
		}
		return funcExpr
	}

	// Check if this is an async arrow function with single parameter: async x => x
	if p.curTokenIs(lexer.IDENT) && p.peekTokenIs(lexer.ARROW) {
		ident := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		p.nextToken() // Move to '=>'
		param := &Parameter{
			Token:          ident.Token,
			Name:           ident,
			TypeAnnotation: nil,
		}
		arrowExpr := p.parseArrowFunctionBodyAndFinish(nil, []*Parameter{param}, nil, nil)
		if arrowLit, ok := arrowExpr.(*ArrowFunctionLiteral); ok {
			arrowLit.IsAsync = true
		}
		return arrowExpr
	}

	// Check if this is an async arrow function with parenthesized parameters: async () => ...
	if p.curTokenIs(lexer.LPAREN) {
		// Save parser state for potential backtracking
		startState := p.l.SaveState()
		startCur := p.curToken
		startPeek := p.peekToken
		startErrors := len(p.errors)

		// parseParameterList expects curToken to be LPAREN
		params, restParam, _ := p.parseParameterList()

		if params != nil && p.curTokenIs(lexer.RPAREN) && p.peekTokenIs(lexer.ARROW) {
			p.nextToken() // Consume ')', cur is now '=>'
			p.errors = p.errors[:startErrors]
			arrowExpr := p.parseArrowFunctionBodyAndFinish(nil, params, restParam, nil)
			if arrowLit, ok := arrowExpr.(*ArrowFunctionLiteral); ok {
				arrowLit.IsAsync = true
			}
			return arrowExpr
		} else if params != nil && p.curTokenIs(lexer.RPAREN) && p.peekTokenIs(lexer.COLON) {
			// async (params): ReturnType => body
			p.nextToken() // Consume ')', cur is now ':'
			p.nextToken() // Consume ':', cur is start of type
			p.errors = p.errors[:startErrors]
			returnTypeAnnotation := p.parseTypeExpression()
			if !p.expectPeek(lexer.ARROW) {
				return nil
			}
			arrowExpr := p.parseArrowFunctionBodyAndFinish(nil, params, restParam, returnTypeAnnotation)
			if arrowLit, ok := arrowExpr.(*ArrowFunctionLiteral); ok {
				arrowLit.IsAsync = true
			}
			return arrowExpr
		} else {
			// Backtrack - not a valid async arrow function
			p.l.RestoreState(startState)
			p.curToken = startCur
			p.peekToken = startPeek
			p.errors = p.errors[:startErrors]
		}
	}

	// If we get here, it's an invalid async expression
	p.addError(asyncToken, fmt.Sprintf("unexpected token after 'async': %s", p.curToken.Type))
	return &Identifier{Token: asyncToken, Value: "async"}
}

// parseAwaitExpression handles await expressions
// await <expression>
// Note: We parse await expressions when await is followed by a valid expression start.
// If await is not followed by a valid expression (e.g., `;`, `,`, `)`), we treat it as an identifier.
// This allows `await` to be used as a variable/parameter name in non-async contexts.
func (p *Parser) parseAwaitExpression() Expression {
	// Check if the next token can start an expression
	// If not, treat 'await' as an identifier instead of an expression keyword
	if !p.canStartExpression(p.peekToken) {
		// 'await' used as identifier in a context where no expression follows
		return &Identifier{
			Token: p.curToken,
			Value: p.curToken.Literal, // "await"
		}
	}

	expression := &AwaitExpression{
		Token: p.curToken, // The 'await' token
	}

	p.nextToken() // Move past 'await'

	// Parse the expression to await with prefix precedence
	// (higher than assignment but lower than most operators)
	expression.Argument = p.parseExpression(PREFIX)
	if expression.Argument == nil {
		p.addError(p.curToken, "expected expression after 'await'")
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

func (p *Parser) parseSatisfiesExpression(left Expression) Expression {
	expression := &SatisfiesExpression{
		Token:      p.curToken, // The 'satisfies' token
		Expression: left,       // The expression being validated
	}

	p.nextToken() // Move past 'satisfies'

	// Parse the target type expression
	expression.TargetType = p.parseTypeExpression()
	if expression.TargetType == nil {
		p.addError(p.curToken, "expected type after 'satisfies'")
		return nil
	}

	return expression
}

// parseGroupedExpression handles expressions like (expr) OR arrow functions like () => expr or (a, b) => expr
func (p *Parser) parseGroupedExpression() Expression {
	// Save complete lexer state for proper backtracking
	startState := p.l.SaveState()
	startCur := p.curToken
	startPeek := p.peekToken
	startErrors := len(p.errors)
	debugPrint("parseGroupedExpression: Starting at pos %d, cur='%s', peek='%s'", startState.Position, startCur.Literal, startPeek.Literal)

	// --- Attempt to parse as Arrow Function Parameters ---
	if p.curTokenIs(lexer.LPAREN) {
		debugPrint("parseGroupedExpression: Attempting arrow param parse...")
		params, restParam, _ := p.parseParameterList() // Consumes up to and including ')'

		// Case 1: Arrow function with params, NO return type annotation: (a, b) => body
		if params != nil && p.curTokenIs(lexer.RPAREN) && p.peekTokenIs(lexer.ARROW) {
			debugPrint("parseGroupedExpression: Successfully parsed arrow params: %v, found '=>' next.", params)
			p.nextToken() // Consume ')', Now curToken is '=>'
			debugPrint("parseGroupedExpression: Consumed ')', cur is now '=>'")
			p.errors = p.errors[:startErrors]                                     // Clear errors from backtrack attempt
			return p.parseArrowFunctionBodyAndFinish(nil, params, restParam, nil) // No return type annotation

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
			return p.parseArrowFunctionBodyAndFinish(nil, params, restParam, returnTypeAnnotation)

		} else {
			// Not an arrow function (or parseParameterList failed), backtrack.
			debugPrint("parseGroupedExpression: Failed arrow param parse (params=%v, cur='%s', peek='%s') or no '=>' or ':', backtracking...", params, p.curToken.Literal, p.peekToken.Type)
			// --- PRECISE BACKTRACK WITH STATE RESTORATION ---
			p.l.RestoreState(startState) // Restore complete lexer state (including template literal state)
			p.curToken = startCur        // Restore original curToken
			p.peekToken = startPeek      // Restore original peekToken
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
		return nil // <<< NIL CHECK
	}
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
		return nil // <<< NIL CHECK
	}
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
				return nil // <<< NIL CHECK
			}
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

// parseCommaExpression handles comma operator expressions like (a, b, c)
func (p *Parser) parseCommaExpression(left Expression) Expression {
	debugPrint("parseCommaExpression: Starting. left=%T('%s'), cur='%s' (%s)", left, left.String(), p.curToken.Literal, p.curToken.Type)

	expression := &InfixExpression{
		Token:    p.curToken, // The comma token
		Operator: p.curToken.Literal,
		Left:     left,
	}

	precedence := p.curPrecedence()
	p.nextToken() // Consume the comma
	expression.Right = p.parseExpression(precedence)

	if expression.Right == nil {
		debugPrint("parseCommaExpression: Right expression was nil, returning nil.")
		return nil
	}

	debugPrint("parseCommaExpression: Finished. Right=%T('%s')", expression.Right, expression.Right.String())
	return expression
}

// parseCallExpression handles function calls like func(arg1, arg2)
// NOTE: Type arguments are handled earlier in the parsing flow
func (p *Parser) parseCallExpression(function Expression) Expression {
	exp := &CallExpression{Token: p.curToken, Function: function}
	exp.Arguments = p.parseExpressionList(lexer.RPAREN)

	// Detect direct eval calls: callee must be a plain Identifier with name "eval"
	// Indirect eval patterns like (0,eval)(...), eval?.(...), or obj.eval(...) won't match
	if ident, ok := function.(*Identifier); ok && ident.Value == "eval" {
		exp.IsDirectEval = true
	}

	return exp
}

// parseTaggedTemplateInfix handles the infix case where peek is TEMPLATE_START after a tag expression
func (p *Parser) parseTaggedTemplateInfix(left Expression) Expression {
	// Current token is TEMPLATE_START (parser advanced before calling infix)
	if !p.curTokenIs(lexer.TEMPLATE_START) {
		return left
	}
	// Parse template literal starting at current TEMPLATE_START
	tmpl := p.parseTemplateLiteral()
	if tmpl == nil {
		return nil
	}
	if tl, ok := tmpl.(*TemplateLiteral); ok {
		return &TaggedTemplateExpression{Token: p.curToken, Tag: left, Template: tl}
	}
	return nil
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
	// Parse with ARG_SEPARATOR precedence - this allows assignment expressions
	// but stops at comma operators (which have lower precedence)
	expr := p.parseExpression(ARG_SEPARATOR)
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
		// Parse with ARG_SEPARATOR precedence - this allows assignment expressions
		// but stops at comma operators (which have lower precedence)
		expr = p.parseExpression(ARG_SEPARATOR)
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
func (p *Parser) parseArrowFunctionBodyAndFinish(typeParams []*TypeParameter, params []*Parameter, restParam *RestParameter, returnTypeAnnotation Expression) Expression {
	debugPrint("parseArrowFunctionBodyAndFinish: Starting, curToken='%s' (%s), params=%v, restParam=%v", p.curToken.Literal, p.curToken.Type, params, restParam)
	arrowFunc := &ArrowFunctionLiteral{
		Token:                p.curToken, // The '=>' token
		TypeParameters:       typeParams, // Use the passed-in type parameters
		Parameters:           params,     // Use the passed-in parameters
		RestParameter:        restParam,  // Use the passed-in rest parameter
		ReturnTypeAnnotation: returnTypeAnnotation,
	}

	p.nextToken() // Consume '=>' ONLY
	debugPrint("parseArrowFunctionBodyAndFinish: Consumed '=>', cur='%s' (%s), peek='%s' (%s)", p.curToken.Literal, p.curToken.Type, p.peekToken.Literal, p.peekToken.Type)

	if p.curTokenIs(lexer.LBRACE) {
		debugPrint("parseArrowFunctionBodyAndFinish: Parsing BlockStatement body...")
		arrowFunc.Body = p.parseBlockStatement() // parseBlockStatement leaves cur at '}'
	} else {
		debugPrint("parseArrowFunctionBodyAndFinish: Parsing Expression body...")
		// No nextToken here - curToken is already the start of the expression
		arrowFunc.Body = p.parseExpression(COMMA)
	}
	debugPrint("parseArrowFunctionBodyAndFinish: Finished parsing body=%T, returning ArrowFunc", arrowFunc.Body)

	// Transform arrow function if it has destructuring parameters
	arrowFunc = p.transformArrowFunctionWithDestructuring(arrowFunc)

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
		return nil, nil, fmt.Errorf("expected '(")
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

	// Parse regular parameter (identifier or destructuring pattern)
	// Allow YIELD as parameter name in non-generator functions (non-strict mode)
	isYieldParam := p.curTokenIs(lexer.YIELD) && p.inGenerator == 0
	// Allow AWAIT as parameter name in non-async functions
	isAwaitParam := p.curTokenIs(lexer.AWAIT) && p.inAsyncFunction == 0
	if !p.curTokenIs(lexer.IDENT) && !p.curTokenIs(lexer.LBRACKET) && !p.curTokenIs(lexer.LBRACE) && !isYieldParam && !isAwaitParam {
		msg := fmt.Sprintf("expected identifier or destructuring pattern as parameter, got %s", p.curToken.Type)
		p.addError(p.curToken, msg)
		debugPrint("parseParameterList: Error - %s", msg)
		return nil, nil, fmt.Errorf("%s", msg)
	}
	param := &Parameter{Token: p.curToken}

	if p.curTokenIs(lexer.LBRACKET) {
		// Array destructuring parameter
		param.IsDestructuring = true
		param.Pattern = p.parseArrayParameterPattern()
		if param.Pattern == nil {
			return nil, nil, fmt.Errorf("failed to parse array parameter pattern")
		}
	} else if p.curTokenIs(lexer.LBRACE) {
		// Object destructuring parameter
		param.IsDestructuring = true
		param.Pattern = p.parseObjectParameterPattern()
		if param.Pattern == nil {
			return nil, nil, fmt.Errorf("failed to parse object parameter pattern")
		}
	} else {
		// Regular identifier parameter
		param.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

	// Check for optional parameter (?)
	if p.peekTokenIs(lexer.QUESTION) {
		if param.IsDestructuring {
			p.addError(p.peekToken, "destructuring parameters cannot be optional")
			return nil, nil, fmt.Errorf("destructuring parameters cannot be optional")
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
		param.TypeAnnotation = nil
	}

	// Check for Default Value
	if p.peekTokenIs(lexer.ASSIGN) {
		if param.IsDestructuring {
			// Allow destructuring parameters to have top-level default values
			// This is valid JavaScript/TypeScript syntax: function f({x} = {}) {}
			p.nextToken() // Consume '='
			p.nextToken() // Move to expression
			param.DefaultValue = p.parseExpression(COMMA)
			if param.DefaultValue == nil {
				return nil, nil, fmt.Errorf("expected expression after '=' in parameter default value")
			}
		} else {
			p.nextToken() // Consume '='
			p.nextToken() // Move to expression
			param.DefaultValue = p.parseExpression(COMMA)
			if param.DefaultValue == nil {
				p.addError(p.curToken, "expected expression after '=' in parameter default value")
				return nil, nil, fmt.Errorf("expected expression after '=' in parameter default value")
			}
		}
	}

	params = append(params, param)
	if param.IsDestructuring {
		debugPrint("parseParameterList: Parsed destructuring param (type: %v)", param.TypeAnnotation)
	} else {
		debugPrint("parseParameterList: Parsed param '%s' (type: %v)", param.Name.Value, param.TypeAnnotation)
	}

	// Parse subsequent parameters (comma-separated)
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken() // Consume ','

		// Check for trailing comma (comma followed by closing paren)
		if p.peekTokenIs(lexer.RPAREN) {
			debugPrint("parseParameterList: Found trailing comma, consuming closing paren")
			p.nextToken() // Consume ')'
			return params, restParam, nil
		}

		p.nextToken() // Consume identifier, destructuring pattern, or spread

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

		// Check for destructuring patterns
		param := &Parameter{Token: p.curToken}
		if p.curTokenIs(lexer.LBRACKET) {
			// Array destructuring parameter: [a, b]
			debugPrint("parseParameterList: Found array destructuring parameter after comma")
			param.Pattern = p.parseArrayParameterPattern()
			if param.Pattern == nil {
				return nil, nil, fmt.Errorf("failed to parse array destructuring parameter after comma")
			}
			param.IsDestructuring = true
		} else if p.curTokenIs(lexer.LBRACE) {
			// Object destructuring parameter: {a, b}
			debugPrint("parseParameterList: Found object destructuring parameter after comma")
			param.Pattern = p.parseObjectParameterPattern()
			if param.Pattern == nil {
				return nil, nil, fmt.Errorf("failed to parse object destructuring parameter after comma")
			}
			param.IsDestructuring = true
		} else if p.curTokenIs(lexer.IDENT) {
			// Regular parameter
			param.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		} else {
			msg := fmt.Sprintf("expected identifier or destructuring pattern for parameter after comma, got %s", p.curToken.Type)
			p.addError(p.curToken, msg)
			debugPrint("parseParameterList: Error - %s", msg)
			return nil, nil, fmt.Errorf("%s", msg)
		}

		// Check for optional parameter (?)
		if p.peekTokenIs(lexer.QUESTION) {
			if param.IsDestructuring {
				p.addError(p.peekToken, "destructuring parameters cannot be optional")
				return nil, nil, fmt.Errorf("destructuring parameters cannot be optional")
			}
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
			if param.IsDestructuring {
				// Allow destructuring parameters to have top-level default values
				// This is valid JavaScript/TypeScript syntax: function f({x} = {}) {}
				p.nextToken() // Consume '='
				p.nextToken() // Move to expression
				param.DefaultValue = p.parseExpression(COMMA)
				if param.DefaultValue == nil {
					return nil, nil, fmt.Errorf("expected expression after '=' in parameter default value")
				}
			} else {
				p.nextToken() // Consume '='
				p.nextToken() // Move to expression
				param.DefaultValue = p.parseExpression(COMMA)
				if param.DefaultValue == nil {
					p.addError(p.curToken, "expected expression after '=' in parameter default value")
					return nil, nil, fmt.Errorf("expected expression after '=' in parameter default value")
				}
			}
		}

		params = append(params, param)
		if param.IsDestructuring {
			patternStr := "<nil>"
			if param.Pattern != nil {
				patternStr = param.Pattern.String()
			}
			debugPrint("parseParameterList: Parsed destructuring param (pattern: %s) (type: %v)", patternStr, param.TypeAnnotation)
		} else {
			debugPrint("parseParameterList: Parsed param '%s' (type: %v)", param.Name.Value, param.TypeAnnotation)
		}
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
	// Use ASSIGNMENT precedence: stops at comma but allows nested ternary (right-associative)
	// COMMA=2 < ASSIGNMENT=4 < TERNARY=5, so comma stops parsing but ternary continues
	debugPrint("parseTernaryExpression parsing alternative...")
	expr.Alternative = p.parseExpression(ASSIGNMENT) // Stop at comma, allow nested ternary
	if expr.Alternative == nil {
		return nil
	} // <<< NIL CHECK
	debugPrint("parseTernaryExpression parsed alternative: %s", expr.Alternative.String())

	debugPrint("parseTernaryExpression finished, returning: %s", expr.String())
	return expr
}

// parseAssignmentExpression handles variable assignment (e.g., x = value)
func (p *Parser) parseAssignmentExpression(left Expression) Expression {
	debugPrint("parseAssignmentExpression starting at line %d:%d with left: %T", p.curToken.Line, p.curToken.Column, left)
	if left == nil {
		debugPrint("parseAssignmentExpression ERROR: left expression is nil!")
		return nil
	}
	debugPrint("parseAssignmentExpression left.String(): %s", left.String())

	// Check for array destructuring assignment: [a, b, c] = expr
	if arrayLit, ok := left.(*ArrayLiteral); ok && p.curToken.Type == lexer.ASSIGN {
		debugPrint("parseAssignmentExpression detected array destructuring pattern")
		return p.parseArrayDestructuringAssignment(arrayLit)
	}

	// Check for object destructuring assignment: {a, b, c} = expr
	if objectLit, ok := left.(*ObjectLiteral); ok && p.curToken.Type == lexer.ASSIGN {
		debugPrint("parseAssignmentExpression detected object destructuring pattern")
		return p.parseObjectDestructuringAssignment(objectLit)
	}

	// Regular assignment expression
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

	// For right-associativity of assignment, parse RHS with ARG_SEPARATOR precedence
	// This allows: a = b = c to parse as a = (b = c) but stops at comma operators
	// ARG_SEPARATOR is between COMMA and ASSIGNMENT, so it excludes comma expressions
	precedence := ARG_SEPARATOR
	p.nextToken() // Consume assignment operator

	debugPrint("parseAssignmentExpression parsing right side...")
	expr.Value = p.parseExpression(precedence)
	if expr.Value == nil {
		debugPrint("parseAssignmentExpression ERROR: right side expression is nil!")
		return nil
	}
	debugPrint("parseAssignmentExpression finished right side: %s (%T)", expr.Value.String(), expr.Value)

	return expr
}

// parseArrayDestructuringAssignment handles array destructuring like [a, b, c] = expr
func (p *Parser) parseArrayDestructuringAssignment(arrayLit *ArrayLiteral) Expression {
	destructure := &ArrayDestructuringAssignment{
		Token: arrayLit.Token, // The '[' token from the array literal
	}

	// Convert array elements to destructuring elements
	for i, element := range arrayLit.Elements {
		var target Expression
		var defaultValue Expression
		var isRest bool

		// Check if this element is a rest element (...rest)
		if spreadExpr, ok := element.(*SpreadElement); ok {
			// This is a rest element: [...rest]
			target = spreadExpr.Argument
			defaultValue = nil
			isRest = true
		} else if assignExpr, ok := element.(*AssignmentExpression); ok && assignExpr.Operator == "=" {
			// This is a default value: [a = 5]
			target = assignExpr.Left
			defaultValue = assignExpr.Value
			isRest = false
		} else {
			// This is a simple element: [a]
			target = element
			defaultValue = nil
			isRest = false
		}

		destElement := &DestructuringElement{
			Target:  target,
			Default: defaultValue,
			IsRest:  isRest,
		}

		// Validate that the target is a valid destructuring target
		if !p.isValidDestructuringTarget(target) {
			msg := fmt.Sprintf("invalid destructuring target: %s (expected identifier, array pattern, or object pattern)", target.String())
			p.addError(arrayLit.Token, msg)
			return nil
		}

		// Validate rest element placement
		if isRest {
			// Rest element must be the last element
			if i != len(arrayLit.Elements)-1 {
				p.addError(arrayLit.Token, "rest element must be last element in destructuring pattern")
				return nil
			}
		}

		destructure.Elements = append(destructure.Elements, destElement)
	}

	// Consume the '=' token (already checked in caller)
	p.nextToken()

	// Parse the right-hand side expression
	destructure.Value = p.parseExpression(LOWEST)
	if destructure.Value == nil {
		p.addError(p.curToken, "expected expression after '=' in array destructuring assignment")
		return nil
	}

	return destructure
}

// parseObjectDestructuringAssignment handles object destructuring like {a, b, c} = expr
func (p *Parser) parseObjectDestructuringAssignment(objectLit *ObjectLiteral) Expression {
	destructure := &ObjectDestructuringAssignment{
		Token: objectLit.Token, // The '{' token from the object literal
	}

	// Convert object properties to destructuring properties
	for i, pair := range objectLit.Properties {
		// Check if this is a spread element (rest property)
		if spreadElement, ok := pair.Key.(*SpreadElement); ok {
			// This is a rest property: ...rest
			if destructure.RestProperty != nil {
				p.addError(objectLit.Token, "multiple rest elements in object destructuring pattern")
				return nil
			}

			// Rest property must be the last property
			if i != len(objectLit.Properties)-1 {
				p.addError(objectLit.Token, "rest element must be last element in object destructuring pattern")
				return nil
			}

			// Validate that the spread argument is an identifier or member expression
			// ECMAScript allows rest targets to be any simple assignment target
			switch target := spreadElement.Argument.(type) {
			case *Identifier:
				destructure.RestProperty = &DestructuringElement{
					Target:  target,
					Default: nil,
					IsRest:  true,
				}
			case *MemberExpression:
				destructure.RestProperty = &DestructuringElement{
					Target:  target,
					Default: nil,
					IsRest:  true,
				}
			case *IndexExpression:
				destructure.RestProperty = &DestructuringElement{
					Target:  target,
					Default: nil,
					IsRest:  true,
				}
			default:
				p.addError(objectLit.Token, "rest element target must be an identifier or member expression")
				return nil
			}
			continue
		}

		// For regular properties, we support simple property destructuring
		// {name, age} = obj (shorthand) or {name: localName} = obj (explicit target)
		// Also support computed keys: {[key]: value} = obj

		var key Expression
		var keyIdent *Identifier
		var target Expression
		var defaultValue Expression

		// Check if the key is an identifier, string/number literal, or computed property name
		if ident, ok := pair.Key.(*Identifier); ok {
			key = ident
			keyIdent = ident
		} else if computedKey, ok := pair.Key.(*ComputedPropertyName); ok {
			key = computedKey
			// Computed keys cannot use shorthand syntax
			keyIdent = nil
		} else if strKey, ok := pair.Key.(*StringLiteral); ok {
			// String literal key like {"foo": x} - convert to identifier for property access
			key = &Identifier{Value: strKey.Value, Token: strKey.Token}
			keyIdent = nil // Cannot use shorthand syntax
		} else if numKey, ok := pair.Key.(*NumberLiteral); ok {
			// Numeric key like {0: x} - convert to identifier for property access
			key = &Identifier{Value: numKey.Token.Literal, Token: numKey.Token}
			keyIdent = nil // Cannot use shorthand syntax
		} else {
			msg := fmt.Sprintf("invalid destructuring property key: %s (expected identifier, string, number, or computed property)", pair.Key.String())
			p.addError(objectLit.Token, msg)
			return nil
		}

		// Check for different patterns:
		// 1. {name} - shorthand without default (only for identifiers)
		// 2. {name = defaultVal} - shorthand with default (only for identifiers)
		// 3. {name: localVar} or {[key]: localVar} - explicit target without default
		// 4. {name: localVar = defaultVal} or {[key]: localVar = defaultVal} - explicit target with default

		if keyIdent != nil {
			// Only identifiers can use shorthand syntax
			if valueIdent, ok := pair.Value.(*Identifier); ok && valueIdent.Value == keyIdent.Value {
				// Pattern 1: Shorthand without default {name}
				target = keyIdent
				defaultValue = nil
			} else if assignExpr, ok := pair.Value.(*AssignmentExpression); ok && assignExpr.Operator == "=" {
				// Check if this is shorthand with default or explicit with default
				if leftIdent, ok := assignExpr.Left.(*Identifier); ok && leftIdent.Value == keyIdent.Value {
					// Pattern 2: Shorthand with default {name = defaultVal}
					target = keyIdent
					defaultValue = assignExpr.Value
				} else {
					// Pattern 4: Explicit target with default {name: localVar = defaultVal}
					target = assignExpr.Left
					defaultValue = assignExpr.Value
				}
			} else {
				// Pattern 3: Explicit target without default {name: localVar}
				target = pair.Value
				defaultValue = nil
			}
		} else {
			// Computed keys must use explicit target syntax
			if assignExpr, ok := pair.Value.(*AssignmentExpression); ok && assignExpr.Operator == "=" {
				// Pattern 4: Computed with default {[key]: localVar = defaultVal}
				target = assignExpr.Left
				defaultValue = assignExpr.Value
			} else {
				// Pattern 3: Computed without default {[key]: localVar}
				target = pair.Value
				defaultValue = nil
			}
		}

		// Validate that the target is a valid destructuring target
		if !p.isValidDestructuringTarget(target) {
			msg := fmt.Sprintf("invalid destructuring target: %s (expected identifier, array pattern, or object pattern)", target.String())
			p.addError(objectLit.Token, msg)
			return nil
		}

		destProperty := &DestructuringProperty{
			Key:     key,
			Target:  target,
			Default: defaultValue,
		}

		destructure.Properties = append(destructure.Properties, destProperty)
	}

	// Consume the '=' token (already checked in caller)
	p.nextToken()

	// Parse the right-hand side expression
	destructure.Value = p.parseExpression(LOWEST)
	if destructure.Value == nil {
		p.addError(p.curToken, "expected expression after '=' in object destructuring assignment")
		return nil
	}

	return destructure
}

// parseArrayDestructuringDeclaration handles let/const/var [a, b] = expr
func (p *Parser) parseArrayDestructuringDeclaration(declToken lexer.Token, isConst bool, requireInitializer bool) *ArrayDestructuringDeclaration {
	decl := &ArrayDestructuringDeclaration{
		Token:   declToken,
		IsConst: isConst,
	}

	// Current token is '[', parse the pattern using similar logic to parseExpressionList
	elements := []Expression{}

	// Check for empty pattern: let [] = ...
	if p.peekTokenIs(lexer.RBRACKET) {
		p.nextToken() // Consume ']'
	} else {
		// Parse elements in a loop, handling elisions
		for {
			p.nextToken() // Move to next position

			// Check for end of array
			if p.curTokenIs(lexer.RBRACKET) {
				break
			}

			// Check for elision (comma without expression before it)
			var element Expression
			if p.curTokenIs(lexer.COMMA) {
				// Elision: [, a] or [a, , b]
				element = nil
				elements = append(elements, element)
				// Don't consume comma - let the loop iteration handle it
			} else {
				// Parse actual element - use ASSIGNMENT precedence to exclude assignment expressions
				// This prevents type checking of `b = 10` as an assignment to undeclared `b`
				element = p.parseExpression(ASSIGNMENT)

				// Check for default value syntax: identifier = defaultExpr
				if p.peekTokenIs(lexer.ASSIGN) {
					p.nextToken() // Consume '='
					p.nextToken() // Move to default value expression
					defaultExpr := p.parseExpression(ASSIGNMENT)
					if defaultExpr == nil {
						return nil
					}
					// Create an AssignmentExpression to represent the default value
					// This won't trigger type checking because we're building it manually
					element = &AssignmentExpression{
						Token:    p.curToken,
						Operator: "=",
						Left:     element,
						Value:    defaultExpr,
					}
				}

				elements = append(elements, element)

				// After parsing element, check what's next
				if p.peekTokenIs(lexer.RBRACKET) {
					p.nextToken() // Consume ']'
					break
				} else if p.peekTokenIs(lexer.COMMA) {
					p.nextToken() // Consume ','
					// Check for trailing comma
					if p.peekTokenIs(lexer.RBRACKET) {
						p.nextToken() // Consume ']'
						break
					}
					// Continue to next element
				} else {
					// Error: expected comma or bracket
					p.addError(p.peekToken, fmt.Sprintf("expected ',' or ']' in array pattern, got %s", p.peekToken.Type))
					return nil
				}
			}
		}
	}

	// Convert elements to DestructuringElements (similar to assignment parsing)
	for i, element := range elements {
		var target Expression
		var defaultValue Expression
		var isRest bool

		// Handle elision (holes in array pattern)
		if element == nil {
			// Elision: [, a] - skip this position
			target = nil
			defaultValue = nil
			isRest = false
		} else if spreadExpr, ok := element.(*SpreadElement); ok {
			// This is a rest element: [...rest]
			target = spreadExpr.Argument

			// Validate that rest element doesn't have a default value
			// Invalid: [...x = []]
			if _, isAssign := target.(*AssignmentExpression); isAssign {
				p.addError(p.curToken, "rest element cannot have a default value")
				return nil
			}

			defaultValue = nil
			isRest = true
		} else if assignExpr, ok := element.(*AssignmentExpression); ok && assignExpr.Operator == "=" {
			target = assignExpr.Left
			defaultValue = assignExpr.Value
			isRest = false
		} else if arrayDestr, ok := element.(*ArrayDestructuringAssignment); ok {
			// Nested array destructuring with default: [[a, b] = [1, 2]]
			// Create an ArrayLiteral to represent the pattern
			arrayPattern := &ArrayLiteral{Token: arrayDestr.Token}
			for _, el := range arrayDestr.Elements {
				arrayPattern.Elements = append(arrayPattern.Elements, el.Target)
			}
			target = arrayPattern
			defaultValue = arrayDestr.Value
			isRest = false
		} else if objDestr, ok := element.(*ObjectDestructuringAssignment); ok {
			// Nested object destructuring with default: [{a, b} = {a: 1, b: 2}]
			// Create an ObjectLiteral to represent the pattern
			objPattern := &ObjectLiteral{Token: objDestr.Token}
			for _, prop := range objDestr.Properties {
				objPattern.Properties = append(objPattern.Properties, &ObjectProperty{
					Key:   prop.Key,
					Value: prop.Target,
				})
			}
			target = objPattern
			defaultValue = objDestr.Value
			isRest = false
		} else {
			target = element
			defaultValue = nil
			isRest = false
		}

		destElement := &DestructuringElement{
			Target:  target,
			Default: defaultValue,
			IsRest:  isRest,
		}

		// Validate that the target is a valid destructuring target (nil is allowed for elision)
		if target != nil && !p.isValidDestructuringTarget(target) {
			p.addError(p.curToken, fmt.Sprintf("invalid destructuring target: %s (expected identifier, array pattern, or object pattern)", target.String()))
			return nil
		}

		// Validate rest element placement
		if isRest {
			// Rest element must be the last element
			if i != len(elements)-1 {
				p.addError(p.curToken, "rest element must be last element in destructuring pattern")
				return nil
			}
		}

		decl.Elements = append(decl.Elements, destElement)
	}

	// Optional type annotation: let [a, b]: [number, string] = ...
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ':'
		p.nextToken() // Move to type expression
		decl.TypeAnnotation = p.parseTypeExpression()
		if decl.TypeAnnotation == nil {
			return nil
		}
	}

	// Require initializer only when explicitly requested (not for for-of/for-in loops)
	if requireInitializer {
		if !p.expectPeek(lexer.ASSIGN) {
			p.addError(p.peekToken, "destructuring declaration must have an initializer")
			return nil
		}

		p.nextToken() // Move to RHS expression
		decl.Value = p.parseExpression(LOWEST)
		if decl.Value == nil {
			return nil
		}

		// Optional semicolon
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken()
		}
	}

	return decl
}

// parseObjectDestructuringPattern parses an object destructuring pattern like {a, b = 2, ...rest}
// Returns the properties and optional rest property
func (p *Parser) parseObjectDestructuringPattern() ([]*DestructuringProperty, *DestructuringElement) {
	if !p.curTokenIs(lexer.LBRACE) {
		p.addError(p.curToken, "expected '{' for object destructuring pattern")
		return nil, nil
	}

	var properties []*DestructuringProperty
	var restProperty *DestructuringElement

	// Handle empty pattern {}
	if p.peekTokenIs(lexer.RBRACE) {
		p.nextToken() // Consume '}'
		// Return empty slice (not nil) to distinguish from error case
		return []*DestructuringProperty{}, nil
	}

	for !p.peekTokenIs(lexer.RBRACE) && !p.peekTokenIs(lexer.EOF) {
		p.nextToken() // Move to property name or spread

		// Check for spread syntax ...rest
		if p.curTokenIs(lexer.SPREAD) {
			p.nextToken() // Consume '...' to get to identifier

			if !p.curTokenIs(lexer.IDENT) {
				p.addError(p.curToken, "expected identifier after '...' in object destructuring")
				return nil, nil
			}

			restProperty = &DestructuringElement{
				Target: &Identifier{Token: p.curToken, Value: p.curToken.Literal},
				IsRest: true,
			}

			// Rest property must be last
			if !p.peekTokenIs(lexer.RBRACE) {
				p.addError(p.curToken, "rest element must be last element in object destructuring pattern")
				return nil, nil
			}

			break
		}

		// Parse property name (identifier, string, number, or computed [expr])
		var propertyKey Expression

		if p.curTokenIs(lexer.LBRACKET) {
			// Computed property name: [expr]
			p.nextToken() // Move past '['
			propertyKey = p.parseExpression(LOWEST)
			if propertyKey == nil {
				return nil, nil
			}
			if !p.expectPeek(lexer.RBRACKET) {
				return nil, nil
			}
			// Wrap in ComputedPropertyName
			propertyKey = &ComputedPropertyName{
				Expr: propertyKey,
			}
		} else if p.curTokenIs(lexer.IDENT) || p.curTokenIs(lexer.STRING) || p.curTokenIs(lexer.NUMBER) || p.curTokenIs(lexer.BIGINT) ||
			// Allow all keywords as property names (JavaScript allows reserved words as property names)
			p.curTokenIs(lexer.BREAK) || p.curTokenIs(lexer.CASE) || p.curTokenIs(lexer.CATCH) ||
			p.curTokenIs(lexer.CLASS) || p.curTokenIs(lexer.CONST) || p.curTokenIs(lexer.CONTINUE) ||
			p.curTokenIs(lexer.DEFAULT) || p.curTokenIs(lexer.DO) || p.curTokenIs(lexer.ELSE) ||
			p.curTokenIs(lexer.EXTENDS) || p.curTokenIs(lexer.FINALLY) || p.curTokenIs(lexer.FOR) ||
			p.curTokenIs(lexer.FUNCTION) || p.curTokenIs(lexer.IF) || p.curTokenIs(lexer.NEW) ||
			p.curTokenIs(lexer.RETURN) || p.curTokenIs(lexer.SWITCH) || p.curTokenIs(lexer.THIS) ||
			p.curTokenIs(lexer.THROW) || p.curTokenIs(lexer.TRY) || p.curTokenIs(lexer.TYPEOF) ||
			p.curTokenIs(lexer.VAR) || p.curTokenIs(lexer.VOID) || p.curTokenIs(lexer.WHILE) ||
			p.curTokenIs(lexer.WITH) || p.curTokenIs(lexer.YIELD) || p.curTokenIs(lexer.GET) ||
			p.curTokenIs(lexer.SET) || p.curTokenIs(lexer.LET) || p.curTokenIs(lexer.AWAIT) ||
			p.curTokenIs(lexer.DELETE) || p.curTokenIs(lexer.IN) || p.curTokenIs(lexer.OF) ||
			p.curTokenIs(lexer.INSTANCEOF) || p.curTokenIs(lexer.STATIC) || p.curTokenIs(lexer.IMPORT) ||
			p.curTokenIs(lexer.EXPORT) || p.curTokenIs(lexer.ASYNC) || p.curTokenIs(lexer.FROM) ||
			p.curTokenIs(lexer.AS) || p.curTokenIs(lexer.NULL) || p.curTokenIs(lexer.TRUE) ||
			p.curTokenIs(lexer.FALSE) || p.curTokenIs(lexer.UNDEFINED) {
			// Regular property name (identifier, keyword, string, number, or bigint)
			if p.curTokenIs(lexer.NUMBER) {
				propertyKey = p.parseNumberLiteral()
			} else if p.curTokenIs(lexer.BIGINT) {
				propertyKey = p.parseBigIntLiteral()
			} else {
				propertyKey = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
				debugPrint("// [PARSER DEBUG] Parsed property key: token=%s, literal=%s\n", p.curToken.Type, p.curToken.Literal)
			}
		} else {
			p.addError(p.curToken, fmt.Sprintf("expected property name, got %s", p.curToken.Type))
			return nil, nil
		}

		property := &DestructuringProperty{
			Key: propertyKey,
		}

		// Check if this is a computed property - computed properties require explicit target
		_, isComputed := propertyKey.(*ComputedPropertyName)

		// Check what follows the property name
		if p.peekTokenIs(lexer.COLON) {
			// Explicit target: { prop: target } or { prop: target = default }
			p.nextToken() // Consume ':'
			p.nextToken() // Move to target

			if p.curTokenIs(lexer.IDENT) {
				property.Target = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
			} else if p.curTokenIs(lexer.LBRACKET) {
				// Nested array destructuring: {prop: [a, b]}
				property.Target = p.parseArrayLiteral()
				if property.Target == nil {
					return nil, nil
				}
			} else if p.curTokenIs(lexer.LBRACE) {
				// Nested object destructuring: {prop: {a, b}}
				property.Target = p.parseObjectLiteral()
				if property.Target == nil {
					return nil, nil
				}
			} else {
				p.addError(p.curToken, fmt.Sprintf("expected identifier, array pattern, or object pattern after ':' in destructuring pattern, got %s", p.curToken.Type))
				return nil, nil
			}

			// Check for default value
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken() // Consume '='
				p.nextToken() // Move to default expression
				property.Default = p.parseExpression(COMMA)
				if property.Default == nil {
					return nil, nil
				}
			}
		} else if isComputed {
			// Computed properties MUST have an explicit target with ':'
			p.addError(p.peekToken, "computed property names in destructuring require an explicit target (e.g., {[expr]: target})")
			return nil, nil
		} else if p.peekTokenIs(lexer.ASSIGN) {
			// Shorthand with default: { prop = default }
			// Only valid for identifier keys
			if ident, ok := propertyKey.(*Identifier); ok {
				property.Target = &Identifier{Token: ident.Token, Value: ident.Value}
				p.nextToken() // Consume '='
				p.nextToken() // Move to default expression
				property.Default = p.parseExpression(COMMA)
				if property.Default == nil {
					return nil, nil
				}
			} else {
				p.addError(p.curToken, "shorthand syntax only works with identifier keys")
				return nil, nil
			}
		} else {
			// Shorthand without default: { prop }
			// Only valid for identifier keys
			if ident, ok := propertyKey.(*Identifier); ok {
				property.Target = &Identifier{Token: ident.Token, Value: ident.Value}
			} else {
				p.addError(p.curToken, "shorthand syntax only works with identifier keys")
				return nil, nil
			}
		}

		properties = append(properties, property)

		// Check for comma or closing brace
		if p.peekTokenIs(lexer.COMMA) {
			p.nextToken() // Consume comma
		} else if !p.peekTokenIs(lexer.RBRACE) {
			p.addError(p.peekToken, fmt.Sprintf("expected ',' or '}' after property in destructuring pattern, got %s", p.peekToken.Type))
			return nil, nil
		}
	}

	if !p.expectPeek(lexer.RBRACE) {
		return nil, nil
	}

	return properties, restProperty
}

// parseObjectDestructuringDeclaration handles let/const/var {a, b} = expr
func (p *Parser) parseObjectDestructuringDeclaration(declToken lexer.Token, isConst bool, requireInitializer bool) *ObjectDestructuringDeclaration {
	decl := &ObjectDestructuringDeclaration{
		Token:   declToken,
		IsConst: isConst,
	}

	// Current token is '{', parse the destructuring pattern
	properties, restProp := p.parseObjectDestructuringPattern()
	if properties == nil && restProp == nil {
		return nil // Error already reported
	}

	decl.Properties = properties
	decl.RestProperty = restProp

	// Optional type annotation: let {a, b}: {a: number, b: string} = ...
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ':'
		p.nextToken() // Move to type expression
		decl.TypeAnnotation = p.parseTypeExpression()
		if decl.TypeAnnotation == nil {
			return nil
		}
	}

	// Require initializer only when explicitly requested (not for for-of/for-in loops)
	if requireInitializer {
		if !p.expectPeek(lexer.ASSIGN) {
			p.addError(p.peekToken, "destructuring declaration must have an initializer")
			return nil
		}

		p.nextToken() // Move to RHS expression
		decl.Value = p.parseExpression(LOWEST)
		if decl.Value == nil {
			return nil
		}

		// Optional semicolon
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken()
		}
	}

	return decl
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

// --- With Statement Parsing ---

func (p *Parser) parseWithStatement() *WithStatement {
	// Parses 'with' '(' <expression> ')' <statement>
	stmt := &WithStatement{Token: p.curToken} // Current token is 'with'

	if !p.expectPeek(lexer.LPAREN) {
		return nil // Expected '(' after 'with'
	}

	p.nextToken() // Consume '('
	stmt.Expression = p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.RPAREN) {
		return nil // Expected ')' after expression
	}

	// Handle both block statements and single statements
	if p.peekTokenIs(lexer.LBRACE) {
		// Block statement case: with (expression) { ... }
		if !p.expectPeek(lexer.LBRACE) {
			return nil
		}
		stmt.Body = p.parseBlockStatement()
	} else {
		// Single statement case: with (expression) statement
		p.nextToken() // Move to the start of the statement
		stmt.Body = p.parseStatement()
		if stmt.Body == nil {
			return nil
		}
	}

	return stmt
}

// --- New: For Statement Parsing ---

func (p *Parser) parseForStatement() Statement {
	// Parse the opening structure first
	forToken := p.curToken

	// Check for 'for await (' syntax (for-await-of loop)
	isAsync := false
	if p.peekTokenIs(lexer.AWAIT) {
		isAsync = true
		p.nextToken() // Consume 'await' token
	}

	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	// Check what comes after the opening paren (and optional 'await') to decide which parser to use
	// We need to peek ahead before advancing

	// Handle empty initializer case: for(; ...)
	if p.peekTokenIs(lexer.SEMICOLON) {
		if isAsync {
			p.addError(p.curToken, "for-await can only be used with for-of loops, not regular for loops")
			return nil
		}
		return p.parseRegularForStatement(forToken)
	}

	// For declaration keywords or identifiers, we need to check if it's for-of/for-in
	// Peek ahead to see the first token after '(' (and optional 'await')
	if p.peekTokenIs(lexer.LET) || p.peekTokenIs(lexer.CONST) || p.peekTokenIs(lexer.VAR) {
		// Advance to the keyword
		p.nextToken()
		return p.parseForStatementOrForOf(forToken, isAsync)
	}

	// For destructuring patterns, check if followed by OF/IN
	if p.peekTokenIs(lexer.LBRACKET) || p.peekTokenIs(lexer.LBRACE) {
		// Advance to the pattern
		p.nextToken() // Now at [ or {
		// Destructuring patterns in for loops are always for-of/for-in (assignment patterns)
		// Parse as expression to handle cases like: for ([x] of items) or for ({a} in obj)
		return p.parseForStatementOrForOf(forToken, isAsync)
	}

	// For identifiers, check if it's followed by OF/IN or could be a member expression
	if p.peekTokenIs(lexer.IDENT) {
		// Save position, advance to check what follows
		p.nextToken() // Now at IDENT

		// Check for member expression patterns: obj.x, obj[x], this.x
		if p.peekTokenIs(lexer.DOT) || p.peekTokenIs(lexer.LBRACKET) {
			// This is a member expression - could be for-of/for-in assignment
			return p.parseForStatementOrForOf(forToken, isAsync)
		}

		if p.peekTokenIs(lexer.OF) || p.peekTokenIs(lexer.IN) {
			return p.parseForStatementOrForOf(forToken, isAsync)
		}
		// Not for-of/for-in, go back and parse as regular for
		// But we can't go back! So we need to stay at IDENT and let parseRegularForStatement handle it
		// Actually, parseRegularForStatement will skip LPAREN check since we're at IDENT
		// So it will parse the ident as an expression
		if isAsync {
			p.addError(p.curToken, "for-await can only be used with for-of loops, not regular for loops")
			return nil
		}
		return p.parseRegularForStatement(forToken)
	}

	// Any other token means it's a regular for loop with expression initializer
	// Don't advance - let parseRegularForStatement handle the LPAREN
	if isAsync {
		p.addError(p.curToken, "for-await can only be used with for-of loops, not regular for loops")
		return nil
	}
	return p.parseRegularForStatement(forToken)
}

// --- New: Break/Continue Statement Parsing ---

func (p *Parser) parseBreakStatement() *BreakStatement {
	stmt := &BreakStatement{Token: p.curToken} // Current token is 'break'
	breakLine := p.curToken.Line

	// Check for optional label - must be on the same line (restricted production)
	if p.peekTokenIs(lexer.IDENT) && p.peekToken.Line == breakLine {
		p.nextToken()
		stmt.Label = &Identifier{
			Token: p.curToken,
			Value: p.curToken.Literal,
		}
	}

	// Consume optional semicolon
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseEmptyStatement() *EmptyStatement {
	stmt := &EmptyStatement{Token: p.curToken} // Current token is ';'
	// Empty statement is just a semicolon, no additional parsing needed
	return stmt
}

func (p *Parser) parseContinueStatement() *ContinueStatement {
	stmt := &ContinueStatement{Token: p.curToken} // Current token is 'continue'
	continueLine := p.curToken.Line

	// Check for optional label - must be on the same line (restricted production)
	if p.peekTokenIs(lexer.IDENT) && p.peekToken.Line == continueLine {
		p.nextToken()
		stmt.Label = &Identifier{
			Token: p.curToken,
			Value: p.curToken.Literal,
		}
	}

	// Consume optional semicolon
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

// --- New: Labeled Statement Parsing ---

func (p *Parser) parseLabeledStatement() *LabeledStatement {
	stmt := &LabeledStatement{Token: p.curToken}

	// Create the label identifier
	stmt.Label = &Identifier{
		Token: p.curToken,
		Value: p.curToken.Literal,
	}

	// Expect and consume the colon
	if !p.expectPeek(lexer.COLON) {
		return nil
	}

	// Parse the labeled statement
	p.nextToken()
	stmt.Statement = p.parseStatement()

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

// parseNonNullExpression parses the non-null assertion operator (x!)
// This is a TypeScript-only operator that asserts a value is not null/undefined
func (p *Parser) parseNonNullExpression(left Expression) Expression {
	expr := &NonNullExpression{
		Token:      p.curToken, // !
		Expression: left,
	}
	// No need to consume token, parseExpression loop does that.
	return expr
}

// --- NEW: Array Literal Parsing ---
func (p *Parser) parseArrayLiteral() Expression {
	array := &ArrayLiteral{Token: p.curToken} // '['
	debugPrint("parseArrayLiteral: start '['")
	// Special-case sparse array literal of the form [,,] or with leading/trailing commas.
	// parseExpressionList expects an element expression before commas; for sparse arrays,
	// treat missing elements as undefined placeholders.
	elements := []Expression{}
	// Advance to first token after '['
	p.nextToken()
	for !p.curTokenIs(lexer.RBRACKET) && !p.curTokenIs(lexer.EOF) {
		if p.curTokenIs(lexer.COMMA) {
			// Elision: push an explicit undefined literal node
			elements = append(elements, &UndefinedLiteral{Token: p.curToken})
			// Consume comma and continue; multiple commas generate multiple holes
			p.nextToken()
			continue
		}
		// Parse element at ASSIGNMENT precedence to exclude assignment expressions
		// This prevents: [x = 10, y = 20] from parsing comma as operator in assignment RHS
		// We manually handle `=` for default values (cover grammar)
		elem := p.parseExpression(ASSIGNMENT)
		if elem == nil {
			return nil
		}

		// Check for default value syntax in array patterns: identifier = defaultExpr
		// This is for cover grammar: [x = 10] can be both array literal and destructuring pattern
		if p.peekTokenIs(lexer.ASSIGN) {
			p.nextToken() // Consume '='
			p.nextToken() // Move to default value expression
			// Parse default expression at ARG_SEPARATOR precedence to allow assignment expressions
			// (e.g., [a = b = c] should parse b = c as the default value for a)
			defaultExpr := p.parseExpression(ARG_SEPARATOR)
			if defaultExpr == nil {
				return nil
			}
			// Create AssignmentExpression to represent the default
			elem = &AssignmentExpression{
				Token:    p.curToken,
				Operator: "=",
				Left:     elem,
				Value:    defaultExpr,
			}
		}

		elements = append(elements, elem)
		debugPrint("parseArrayLiteral: appended element=%T ('%s')", elem, elem.String())
		// Optional comma between elements
		if p.peekTokenIs(lexer.COMMA) {
			p.nextToken() // move to comma
			p.nextToken() // move past comma
			debugPrint("parseArrayLiteral: consumed ',', cur='%s'", p.curToken.Literal)
			continue
		}
		// Otherwise, expect ']' next
		if !p.peekTokenIs(lexer.RBRACKET) {
			p.addError(p.curToken, "expected ',' or ']' in array literal")
			return nil
		}
		p.nextToken() // move to ']'
	}
	debugPrint("parseArrayLiteral: end ']' elements=%d", len(elements))
	if !p.curTokenIs(lexer.RBRACKET) {
		p.peekError(lexer.RBRACKET)
		return nil
	}
	array.Elements = elements
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
// Limits the number of errors to prevent memory exhaustion from infinite parsing loops.
func (p *Parser) addError(tok lexer.Token, msg string) {
	// Prevent memory exhaustion from infinite error generation
	const maxErrors = 1000
	if len(p.errors) >= maxErrors {
		if len(p.errors) == maxErrors {
			// Add one final error indicating we hit the limit
			syntaxErr := &errors.SyntaxError{
				Position: errors.Position{
					Line:     tok.Line,
					Column:   tok.Column,
					StartPos: tok.StartPos,
					EndPos:   tok.EndPos,
					Source:   p.source,
				},
				Msg: fmt.Sprintf("too many parse errors (limit: %d), stopping parser", maxErrors),
			}
			p.errors = append(p.errors, syntaxErr)
		}
		return // Stop adding more errors
	}

	syntaxErr := &errors.SyntaxError{
		Position: errors.Position{
			Line:     tok.Line,
			Column:   tok.Column,
			StartPos: tok.StartPos,
			EndPos:   tok.EndPos,
			Source:   p.source, // Use parser's cached source context
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
	caseClause.Body.HoistedDeclarations = make(map[string]Expression) // Initialize for function hoisting

	// Loop until the next case, default, or the end of the switch block
	// Similar loop logic as parseBlockStatement
	for !p.curTokenIs(lexer.CASE) && !p.curTokenIs(lexer.DEFAULT) && !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
		stmt := p.parseStatement() // parseStatement consumes tokens including optional semicolon
		if stmt != nil {
			caseClause.Body.Statements = append(caseClause.Body.Statements, stmt)

			// --- Hoisting Check (same as parseBlockStatement) ---
			// Check if the statement IS an ExpressionStatement containing a FunctionLiteral
			if exprStmt, isExprStmt := stmt.(*ExpressionStatement); isExprStmt && exprStmt.Expression != nil {
				if funcLit, isFuncLit := exprStmt.Expression.(*FunctionLiteral); isFuncLit && funcLit.Name != nil {
					if _, exists := caseClause.Body.HoistedDeclarations[funcLit.Name.Value]; exists {
						// Function with this name already hoisted in this block
						p.addError(funcLit.Name.Token, fmt.Sprintf("duplicate hoisted function declaration in switch case: %s", funcLit.Name.Value))
					} else {
						caseClause.Body.HoistedDeclarations[funcLit.Name.Value] = funcLit
					}
				}
			}
			// --- End Hoisting Check ---
		} else {
			// If parseStatement returns nil due to an error, break the inner loop
			// to avoid infinite loops and let the outer switch parser handle recovery.
			// An error message should have already been added by parseStatement or its children.
			break
		}

		// Advance AFTER parsing the statement, similar to parseBlockStatement
		// IMPORTANT: Call nextToken BEFORE checking for termination, because after parsing
		// a statement with a block body (like if/while/for), curToken will be '}' from that
		// inner block, not from the switch. We need to advance past it first.
		p.nextToken()
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

	// Save the identifier
	ident := &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check if this could be a generic type reference
	// We need to be careful here - only parse as generic if we see '<' followed by a valid type
	if p.peekTokenIs(lexer.LT) {
		// Try to parse as generic, but be ready to backtrack
		return p.tryParseGenericTypeRef(ident)
	}

	return ident
}

// tryParseGenericTypeRef attempts to parse Array<T> syntax
// If it fails, it returns the original identifier
func (p *Parser) tryParseGenericTypeRef(name *Identifier) Expression {
	// For now, we'll use a simpler approach without full backtracking
	// This is safe because we're only in type context

	// Consume the '<'
	p.nextToken()
	if !p.curTokenIs(lexer.LT) {
		// This shouldn't happen since we checked peek
		return name
	}

	// Try to parse type arguments
	var typeArgs []Expression

	// Parse first type argument
	p.nextToken()
	if p.curTokenIs(lexer.GT) {
		// Empty type arguments not allowed
		p.addError(p.curToken, "Expected type argument after '<'")
		return name
	}

	firstArg := p.parseTypeExpression()
	if firstArg == nil {
		// Error already added by parseTypeExpression
		return name
	}
	typeArgs = append(typeArgs, firstArg)

	// Parse remaining type arguments
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken() // consume comma
		p.nextToken() // move to next type

		arg := p.parseTypeExpression()
		if arg == nil {
			// Error already added by parseTypeExpression
			return name
		}
		typeArgs = append(typeArgs, arg)
	}

	// Expect closing '>' - handle >> and >>> splitting
	if !p.expectPeekGT() {
		return name
	}

	// Success! Create generic type ref
	return &GenericTypeRef{
		BaseExpression: BaseExpression{},
		Token:          name.Token, // Use the identifier token
		Name:           name,
		TypeArguments:  typeArgs,
	}
}
func (p *Parser) parseObjectLiteral() Expression {
	objLit := &ObjectLiteral{
		Token: p.curToken, // The '{' token
		// --- MODIFIED: Initialize slice ---
		Properties: []*ObjectProperty{},
	}
	for !p.peekTokenIs(lexer.RBRACE) && !p.peekTokenIs(lexer.EOF) {
		p.nextToken() // Consume '{' or ',' to get to the key

		// --- NEW: Check for spread syntax (...expression) ---
		if p.curTokenIs(lexer.SPREAD) {
			// Parse spread element: ...expression
			spreadToken := p.curToken
			debugPrint("Object spread: starting at token %s, peek is %s", p.curToken.Type, p.peekToken.Type)
			p.nextToken() // Consume '...' to get to the expression
			debugPrint("Object spread: after consuming '...', cur=%s(%s), peek=%s(%s)",
				p.curToken.Type, p.curToken.Literal, p.peekToken.Type, p.peekToken.Literal)

			// Parse the expression being spread
			// Use ASSIGNMENT precedence to stop before commas (don't consume comma operator)
			spreadExpr := p.parseExpression(ASSIGNMENT)
			debugPrint("Object spread: after parseExpression(COMMA), cur=%s(%s), peek=%s(%s)",
				p.curToken.Type, p.curToken.Literal, p.peekToken.Type, p.peekToken.Literal)
			if spreadExpr == nil {
				p.addError(p.curToken, "expected expression after '...' in object literal")
				return nil
			}

			// Create a SpreadElement
			spreadElement := &SpreadElement{
				Token:    spreadToken,
				Argument: spreadExpr,
			}

			// Add as a special property where Key is SpreadElement and Value is nil
			objLit.Properties = append(objLit.Properties, &ObjectProperty{
				Key:   spreadElement,
				Value: nil, // No separate value for spread elements
			})

			// Check for comma or closing brace
			debugPrint("Object spread: checking next token - peek=%s(%s)", p.peekToken.Type, p.peekToken.Literal)
			if p.peekTokenIs(lexer.COMMA) {
				debugPrint("Object spread: found comma, consuming it")
				p.nextToken() // Consume comma
				continue
			} else if p.peekTokenIs(lexer.RBRACE) {
				debugPrint("Object spread: found rbrace, breaking")
				break // End of object
			} else {
				debugPrint("Object spread: ERROR - expected comma or rbrace but got %s(%s)", p.peekToken.Type, p.peekToken.Literal)
				p.addError(p.curToken, "expected ',' or '}' after spread element")
				return nil
			}
		}

		// --- NEW: Check for getters and setters (including computed ones) ---
		if p.curTokenIs(lexer.GET) && p.isGetterMethod() {
			// Parse getter inline: get propertyName() { ... } or get [computed]() { ... }
			getToken := p.curToken // 'get' token
			p.nextToken()          // Move to property name

			var key Expression
			if p.curTokenIs(lexer.LBRACKET) {
				// Computed getter: get [expr]() { ... }
				p.nextToken() // Consume '['
				keyExpr := p.parseExpression(COMMA)
				if keyExpr == nil {
					return nil
				}
				if !p.expectPeek(lexer.RBRACKET) {
					return nil
				}
				key = &ComputedPropertyName{Expr: keyExpr}
			} else if p.curTokenIs(lexer.STRING) {
				key = &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
			} else if p.curTokenIs(lexer.NUMBER) {
				key = p.parseNumberLiteral()
			} else if p.curTokenIs(lexer.BIGINT) {
				key = p.parseBigIntLiteral()
			} else {
				// Try to parse as property name (handles IDENT and all keywords)
				key = p.parsePropertyName()
				if key == nil {
					p.addError(p.curToken, "expected identifier, string literal, number, computed property, or keyword after 'get'")
					return nil
				}
			}

			// Expect '(' for getter function
			if !p.expectPeek(lexer.LPAREN) {
				return nil
			}

			// Parse getter function (should have no parameters)
			funcLit := &FunctionLiteral{Token: getToken}
			funcLit.Parameters, funcLit.RestParameter, _ = p.parseFunctionParameters(false)
			if funcLit.Parameters == nil && funcLit.RestParameter == nil {
				return nil
			}

			// Validate that getters have no parameters
			if len(funcLit.Parameters) > 0 || funcLit.RestParameter != nil {
				p.addError(p.curToken, "getters cannot have parameters")
				return nil
			}

			// Optional return type annotation
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken()
				p.nextToken()
				funcLit.ReturnTypeAnnotation = p.parseTypeExpression()
				if funcLit.ReturnTypeAnnotation == nil {
					return nil
				}
			}

			// Expect '{' for body
			if !p.expectPeek(lexer.LBRACE) {
				return nil
			}

			// Parse function body
			funcLit.Body = p.parseBlockStatement()
			if funcLit.Body == nil {
				return nil
			}

			// Transform function if it has destructuring parameters
			funcLit = p.transformFunctionWithDestructuring(funcLit)

			// Create MethodDefinition for getter
			getter := &MethodDefinition{
				Token:       getToken,
				Key:         key,
				Value:       funcLit,
				Kind:        "getter",
				IsStatic:    false,
				IsPublic:    false,
				IsPrivate:   false,
				IsProtected: false,
				IsOverride:  false,
			}

			// Add getter as ObjectProperty with MethodDefinition as value
			objLit.Properties = append(objLit.Properties, &ObjectProperty{
				Key:   key,
				Value: getter,
			})

		} else if p.curTokenIs(lexer.SET) && p.isSetterMethod() {
			// Parse setter inline: set propertyName(param) { ... } or set [computed](param) { ... }
			setToken := p.curToken // 'set' token
			p.nextToken()          // Move to property name

			var key Expression
			if p.curTokenIs(lexer.LBRACKET) {
				// Computed setter: set [expr](param) { ... }
				p.nextToken() // Consume '['
				keyExpr := p.parseExpression(COMMA)
				if keyExpr == nil {
					return nil
				}
				if !p.expectPeek(lexer.RBRACKET) {
					return nil
				}
				key = &ComputedPropertyName{Expr: keyExpr}
			} else if p.curTokenIs(lexer.STRING) {
				key = &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
			} else if p.curTokenIs(lexer.NUMBER) {
				key = p.parseNumberLiteral()
			} else if p.curTokenIs(lexer.BIGINT) {
				key = p.parseBigIntLiteral()
			} else {
				// Try to parse as property name (handles IDENT and all keywords)
				key = p.parsePropertyName()
				if key == nil {
					p.addError(p.curToken, "expected identifier, string literal, number, computed property, or keyword after 'set'")
					return nil
				}
			}

			// Expect '(' for setter function
			if !p.expectPeek(lexer.LPAREN) {
				return nil
			}

			// Parse setter function (should have exactly one parameter)
			funcLit := &FunctionLiteral{Token: setToken}
			funcLit.Parameters, funcLit.RestParameter, _ = p.parseFunctionParameters(false)
			if funcLit.Parameters == nil && funcLit.RestParameter == nil {
				return nil
			}

			// Validate that setters have exactly one parameter
			if len(funcLit.Parameters) != 1 || funcLit.RestParameter != nil {
				p.addError(p.curToken, "setters must have exactly one parameter")
				return nil
			}

			// Optional return type annotation
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken()
				p.nextToken()
				funcLit.ReturnTypeAnnotation = p.parseTypeExpression()
				if funcLit.ReturnTypeAnnotation == nil {
					return nil
				}
			}

			// Expect '{' for body
			if !p.expectPeek(lexer.LBRACE) {
				return nil
			}

			// Parse function body
			funcLit.Body = p.parseBlockStatement()
			if funcLit.Body == nil {
				return nil
			}

			// Transform function if it has destructuring parameters
			funcLit = p.transformFunctionWithDestructuring(funcLit)

			// Create MethodDefinition for setter
			setter := &MethodDefinition{
				Token:       setToken,
				Key:         key,
				Value:       funcLit,
				Kind:        "setter",
				IsStatic:    false,
				IsPublic:    false,
				IsPrivate:   false,
				IsProtected: false,
				IsOverride:  false,
			}

			// Add setter as ObjectProperty with MethodDefinition as value
			objLit.Properties = append(objLit.Properties, &ObjectProperty{
				Key:   key,
				Value: setter,
			})

		} else if p.curTokenIs(lexer.LBRACKET) {
			// Computed property: [expression]: value
			p.nextToken() // Consume '['
			key := p.parseExpression(COMMA)
			if key == nil {
				return nil // Error parsing expression inside []
			}
			if !p.expectPeek(lexer.RBRACKET) {
				return nil // Missing closing ']'
			}

			// Wrap in ComputedPropertyName node
			computedKey := &ComputedPropertyName{
				Expr: key,
			}

			// Check if this is a computed method: [expr]() { ... }
			if p.peekTokenIs(lexer.LPAREN) {
				// Parse as computed method - create a function literal
				funcLit := &FunctionLiteral{
					Token: p.curToken, // Current token (should be after ']')
				}

				// Expect '(' for parameters
				if !p.expectPeek(lexer.LPAREN) {
					return nil
				}

				// Parse parameters
				funcLit.Parameters, funcLit.RestParameter, _ = p.parseFunctionParameters(false)
				if funcLit.Parameters == nil && funcLit.RestParameter == nil {
					return nil // Error parsing parameters
				}

				// Check for optional return type annotation
				if p.peekTokenIs(lexer.COLON) {
					p.nextToken() // Consume ')'
					p.nextToken() // Consume ':'
					funcLit.ReturnTypeAnnotation = p.parseTypeExpression()
					if funcLit.ReturnTypeAnnotation == nil {
						return nil // Error parsing return type
					}
				}

				// Expect '{' for body
				if !p.expectPeek(lexer.LBRACE) {
					return nil
				}

				// Parse function body
				funcLit.Body = p.parseBlockStatement()
				if funcLit.Body == nil {
					return nil
				}

				// Transform function if it has destructuring parameters
				funcLit = p.transformFunctionWithDestructuring(funcLit)

				// Add the computed method
				objLit.Properties = append(objLit.Properties, &ObjectProperty{
					Key:   computedKey,
					Value: funcLit,
				})
			} else {
				// Regular computed property: [expr]: value
				// Expect colon
				if !p.expectPeek(lexer.COLON) {
					return nil
				}

				// Parse the value
				p.nextToken()
				value := p.parseExpression(COMMA)
				if value == nil {
					return nil
				}

				// Add the computed property
				objLit.Properties = append(objLit.Properties, &ObjectProperty{
					Key:   computedKey,
					Value: value,
				})
			}
		} else {
			// --- NEW: Check for async methods/generators (async foo() or async *foo()) ---
			if p.curTokenIs(lexer.ASYNC) {
				asyncToken := p.curToken
				p.nextToken() // Consume 'async' to see what's next

				// Check if this is an async generator (async *foo())
				isAsyncGenerator := p.curTokenIs(lexer.ASTERISK)
				if isAsyncGenerator {
					p.nextToken() // Consume '*' to get to the name
				}

				var key Expression
				var funcLit *FunctionLiteral

				// Handle different name types
				if p.curTokenIs(lexer.IDENT) || p.curTokenIs(lexer.YIELD) ||
					p.curTokenIs(lexer.GET) || p.curTokenIs(lexer.SET) ||
					p.curTokenIs(lexer.THROW) || p.curTokenIs(lexer.RETURN) ||
					p.curTokenIs(lexer.LET) || p.curTokenIs(lexer.AWAIT) {
					key = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
					funcLit = &FunctionLiteral{
						Token:       asyncToken,
						IsAsync:     true,
						IsGenerator: isAsyncGenerator,
					}
				} else if p.curTokenIs(lexer.STRING) {
					key = &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
					funcLit = &FunctionLiteral{
						Token:       asyncToken,
						IsAsync:     true,
						IsGenerator: isAsyncGenerator,
					}
				} else if p.curTokenIs(lexer.LBRACKET) {
					// async [Symbol.asyncIterator]() { ... }
					p.nextToken() // Consume '['
					keyExpr := p.parseExpression(COMMA)
					if keyExpr == nil {
						return nil
					}
					if !p.expectPeek(lexer.RBRACKET) {
						return nil
					}
					key = &ComputedPropertyName{Expr: keyExpr}
					funcLit = &FunctionLiteral{
						Token:       asyncToken,
						IsAsync:     true,
						IsGenerator: isAsyncGenerator,
					}
				} else {
					p.addError(p.curToken, "expected identifier, string literal, or computed property name after 'async' in async method")
					return nil
				}

				// Expect '(' for parameters
				if !p.expectPeek(lexer.LPAREN) {
					return nil
				}

				// Parse parameters
				funcLit.Parameters, funcLit.RestParameter, _ = p.parseFunctionParameters(false)
				if funcLit.Parameters == nil && funcLit.RestParameter == nil {
					return nil
				}

				// Check for optional return type annotation
				if p.peekTokenIs(lexer.COLON) {
					p.nextToken() // Consume ')'
					p.nextToken() // Consume ':'
					funcLit.ReturnTypeAnnotation = p.parseTypeExpression()
					if funcLit.ReturnTypeAnnotation == nil {
						return nil
					}
				}

				// Expect '{' for method body
				if !p.expectPeek(lexer.LBRACE) {
					return nil
				}

				// Save and manage generator context for async generators
				savedGeneratorContext := p.inGenerator
				if funcLit.IsGenerator {
					p.inGenerator++
					if debugParser {
						fmt.Printf("[PARSER] Entering generator context (async generator method), inGenerator=%d\n", p.inGenerator)
					}
				}

				// Parse method body
				funcLit.Body = p.parseBlockStatement()
				if funcLit.Body == nil {
					return nil
				}

				// Restore generator context
				if funcLit.IsGenerator {
					p.inGenerator = savedGeneratorContext
					if debugParser {
						fmt.Printf("[PARSER] Restored generator context to %d (async generator method)\n", p.inGenerator)
					}
				}

				// Transform function if it has destructuring parameters
				funcLit = p.transformFunctionWithDestructuring(funcLit)

				// Add the async method
				objLit.Properties = append(objLit.Properties, &ObjectProperty{
					Key:   key,
					Value: funcLit,
				})
			} else if p.curTokenIs(lexer.ASTERISK) {
				// --- NEW: Check for generator methods (*foo() or *"foo"() or *[expr]()) ---
				// This is a generator method
				asteriskToken := p.curToken
				p.nextToken() // Consume '*' to get to the name

				var key Expression
				var funcLit *FunctionLiteral

				// Handle different name types for generator methods
				if p.curTokenIs(lexer.IDENT) || p.curTokenIs(lexer.YIELD) ||
					p.curTokenIs(lexer.GET) || p.curTokenIs(lexer.SET) ||
					p.curTokenIs(lexer.THROW) || p.curTokenIs(lexer.RETURN) ||
					p.curTokenIs(lexer.LET) || p.curTokenIs(lexer.AWAIT) {
					// *foo() or *yield() or other contextual keywords
					key = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
					funcLit = &FunctionLiteral{
						Token:       asteriskToken,
						IsGenerator: true,
					}
				} else if p.curTokenIs(lexer.STRING) {
					// *"foo"() { ... }
					key = &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
					funcLit = &FunctionLiteral{
						Token:       asteriskToken,
						IsGenerator: true,
					}
				} else if p.curTokenIs(lexer.LBRACKET) {
					// *[Symbol.iterator]() { ... }
					p.nextToken() // Consume '['
					keyExpr := p.parseExpression(COMMA)
					if keyExpr == nil {
						return nil // Error parsing expression inside []
					}
					if !p.expectPeek(lexer.RBRACKET) {
						return nil // Missing closing ']'
					}

					// Wrap in ComputedPropertyName node
					key = &ComputedPropertyName{
						Expr: keyExpr,
					}
					funcLit = &FunctionLiteral{
						Token:       asteriskToken,
						IsGenerator: true,
					}
				} else {
					p.addError(p.curToken, "expected identifier, string literal, or computed property name after '*' in generator method")
					return nil
				}

				// Expect '(' for parameters
				if !p.expectPeek(lexer.LPAREN) {
					return nil
				}

				// Parse parameters
				funcLit.Parameters, funcLit.RestParameter, _ = p.parseFunctionParameters(false)
				if funcLit.Parameters == nil && funcLit.RestParameter == nil {
					return nil // Error parsing parameters
				}

				// Check for optional return type annotation
				if p.peekTokenIs(lexer.COLON) {
					p.nextToken() // Consume ')'
					p.nextToken() // Consume ':'
					funcLit.ReturnTypeAnnotation = p.parseTypeExpression()
					if funcLit.ReturnTypeAnnotation == nil {
						return nil // Error parsing return type
					}
				}

				// Expect '{' for method body
				if !p.expectPeek(lexer.LBRACE) {
					return nil
				}

				// Save and manage generator context (same as parseFunctionLiteral)
				savedGeneratorContext := p.inGenerator
				if funcLit.IsGenerator {
					p.inGenerator++
					if debugParser {
						fmt.Printf("[PARSER] Entering generator context (object method), inGenerator=%d\n", p.inGenerator)
					}
				} else {
					// Non-generator method resets generator context
					p.inGenerator = 0
					if debugParser && savedGeneratorContext > 0 {
						fmt.Printf("[PARSER] Resetting generator context for non-generator method (was %d)\n", savedGeneratorContext)
					}
				}

				// Parse method body
				funcLit.Body = p.parseBlockStatement()
				if funcLit.Body == nil {
					return nil // Error parsing method body
				}

				// Restore the saved generator context
				p.inGenerator = savedGeneratorContext
				if debugParser {
					fmt.Printf("[PARSER] Restored generator context to %d (object method)\n", p.inGenerator)
				}

				// Transform function if it has destructuring parameters
				funcLit = p.transformFunctionWithDestructuring(funcLit)

				// Create MethodDefinition for generator method
				// This ensures [[HomeObject]] is set for super property access
				generatorMethod := &MethodDefinition{
					Token:       asteriskToken,
					Key:         key,
					Value:       funcLit,
					Kind:        "method", // Generator methods have kind "method", not "generator"
					IsStatic:    false,
					IsPublic:    false,
					IsPrivate:   false,
					IsProtected: false,
					IsOverride:  false,
				}

				// Add the generator method
				objLit.Properties = append(objLit.Properties, &ObjectProperty{
					Key:   key,
					Value: generatorMethod,
				})
			} else if p.curTokenIs(lexer.STRING) && p.peekTokenIs(lexer.LPAREN) {
				// This is a string literal shorthand method like "methodName"() { ... }
				stringKey := &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}

				// Create a function literal for the method implementation
				funcLit := &FunctionLiteral{
					Token: p.curToken, // The string literal token
				}

				// Expect '(' for parameters
				if !p.expectPeek(lexer.LPAREN) {
					return nil
				}

				// Parse parameters
				funcLit.Parameters, funcLit.RestParameter, _ = p.parseFunctionParameters(false)
				if funcLit.Parameters == nil && funcLit.RestParameter == nil {
					return nil // Error parsing parameters
				}

				// Check for optional return type annotation
				if p.peekTokenIs(lexer.COLON) {
					p.nextToken() // Consume ')'
					p.nextToken() // Consume ':'
					funcLit.ReturnTypeAnnotation = p.parseTypeExpression()
					if funcLit.ReturnTypeAnnotation == nil {
						return nil // Error parsing return type
					}
				}

				// Expect '{' for method body
				if !p.expectPeek(lexer.LBRACE) {
					return nil
				}

				// Parse method body
				funcLit.Body = p.parseBlockStatement()
				if funcLit.Body == nil {
					return nil // Error parsing method body
				}

				// Transform function if it has destructuring parameters
				funcLit = p.transformFunctionWithDestructuring(funcLit)

				// Create an ObjectProperty with the string literal as key and the function literal as value
				objLit.Properties = append(objLit.Properties, &ObjectProperty{Key: stringKey, Value: funcLit})
			} else if p.curTokenIs(lexer.NUMBER) && p.peekTokenIs(lexer.LPAREN) {
				// This is a number literal shorthand method like 1() { ... }
				numberKey := p.parseNumberLiteral()

				// Create a function literal for the method implementation
				funcLit := &FunctionLiteral{
					Token: p.curToken, // The number literal token
				}

				// Expect '(' for parameters
				if !p.expectPeek(lexer.LPAREN) {
					return nil
				}

				// Parse parameters
				funcLit.Parameters, funcLit.RestParameter, _ = p.parseFunctionParameters(false)
				if funcLit.Parameters == nil && funcLit.RestParameter == nil {
					return nil // Error parsing parameters
				}

				// Check for optional return type annotation
				if p.peekTokenIs(lexer.COLON) {
					p.nextToken() // Consume ')'
					p.nextToken() // Consume ':'
					funcLit.ReturnTypeAnnotation = p.parseTypeExpression()
					if funcLit.ReturnTypeAnnotation == nil {
						return nil // Error parsing return type
					}
				}

				// Expect '{' for method body
				if !p.expectPeek(lexer.LBRACE) {
					return nil
				}

				// Parse method body
				funcLit.Body = p.parseBlockStatement()
				if funcLit.Body == nil {
					return nil // Error parsing method body
				}

				// Transform function if it has destructuring parameters
				funcLit = p.transformFunctionWithDestructuring(funcLit)

				// Create an ObjectProperty with the number literal as key and the function literal as value
				objLit.Properties = append(objLit.Properties, &ObjectProperty{Key: numberKey, Value: funcLit})
			} else if p.curTokenIs(lexer.BIGINT) && p.peekTokenIs(lexer.LPAREN) {
				// This is a bigint literal shorthand method like 1n() { ... }
				bigintKey := p.parseBigIntLiteral()

				// Create a function literal for the method implementation
				funcLit := &FunctionLiteral{
					Token: p.curToken, // The bigint literal token
				}

				// Expect '(' for parameters
				if !p.expectPeek(lexer.LPAREN) {
					return nil
				}

				// Parse parameters
				funcLit.Parameters, funcLit.RestParameter, _ = p.parseFunctionParameters(false)
				if funcLit.Parameters == nil && funcLit.RestParameter == nil {
					return nil // Error parsing parameters
				}

				// Check for optional return type annotation
				if p.peekTokenIs(lexer.COLON) {
					p.nextToken() // Consume ')'
					p.nextToken() // Consume ':'
					funcLit.ReturnTypeAnnotation = p.parseTypeExpression()
					if funcLit.ReturnTypeAnnotation == nil {
						return nil // Error parsing return type
					}
				}

				// Expect '{' for method body
				if !p.expectPeek(lexer.LBRACE) {
					return nil
				}

				// Parse method body
				funcLit.Body = p.parseBlockStatement()
				if funcLit.Body == nil {
					return nil // Error parsing method body
				}

				// Transform function if it has destructuring parameters
				funcLit = p.transformFunctionWithDestructuring(funcLit)

				// Create an ObjectProperty with the bigint literal as key and the function literal as value
				objLit.Properties = append(objLit.Properties, &ObjectProperty{Key: bigintKey, Value: funcLit})
			} else {
				// --- NEW: Check for shorthand method syntax (identifier/keyword followed by '(') ---
				propName := p.parsePropertyName()
				if propName != nil && p.peekTokenIs(lexer.LPAREN) {
					// This is a shorthand method like methodName() { ... }
					methodDef := p.parseShorthandMethod()
					if methodDef == nil {
						return nil // Error parsing shorthand method
					}

					// Create an ObjectProperty with the method name as key and the MethodDefinition as value
					objLit.Properties = append(objLit.Properties, &ObjectProperty{Key: methodDef.Key, Value: methodDef})
				} else if propName != nil && (p.peekTokenIs(lexer.COMMA) || p.peekTokenIs(lexer.RBRACE) || p.peekTokenIs(lexer.ASSIGN)) {
					// --- NEW: Check for shorthand property syntax ---
					// This handles:
					// 1. { name, age } - shorthand without default
					// 2. { name = 5, age = 10 } - shorthand with default (for destructuring)
					identName := p.curToken.Literal
					key := propName

					var value Expression
					if p.peekTokenIs(lexer.ASSIGN) {
						// Shorthand with default: { x = 5 }
						// Create an assignment expression: x = 5
						// This will be interpreted as default value in destructuring context
						identToken := p.curToken // Save the identifier token before advancing
						p.nextToken()            // Consume '='
						assignToken := p.curToken
						p.nextToken() // Move to default value expression

						defaultValue := p.parseExpression(COMMA)
						if defaultValue == nil {
							return nil
						}

						// Create assignment expression for destructuring: x = defaultValue
						value = &AssignmentExpression{
							Token:    assignToken,
							Operator: "=",
							Left:     &Identifier{Token: identToken, Value: identName},
							Value:    defaultValue,
						}
					} else {
						// Shorthand without default: { x }
						value = &Identifier{Token: p.curToken, Value: identName}
					}

					// Append the property
					objLit.Properties = append(objLit.Properties, &ObjectProperty{Key: key, Value: value})
				} else {
					// Regular property parsing
					var key Expression
					// --- MODIFIED: Handle Keys (Identifier/Keywords, String, NUMBER) ---
					if propName != nil {
						key = propName
					} else if p.curTokenIs(lexer.STRING) {
						key = p.parseStringLiteral()
					} else if p.curTokenIs(lexer.NUMBER) {
						key = p.parseNumberLiteral()
					} else if p.curTokenIs(lexer.BIGINT) {
						key = p.parseBigIntLiteral()
					} else {
						msg := fmt.Sprintf("invalid object literal key: expected identifier, string, number, bigint, or '[', got %s", p.curToken.Type)
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

					// Use COMMA precedence to allow assignment expressions (for destructuring defaults)
					// while still stopping at commas (for next property)
					value := p.parseExpression(COMMA)
					if value == nil {
						return nil
					} // Error parsing value

					// Append the property
					objLit.Properties = append(objLit.Properties, &ObjectProperty{Key: key, Value: value})
				}
			}
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
func (p *Parser) parseShorthandMethod() *MethodDefinition {
	methodToken := p.curToken
	methodName := p.parsePropertyName()
	if methodName == nil {
		p.addError(p.curToken, "expected method name (identifier) for shorthand method")
		return nil
	}

	// Create a function literal for the method implementation
	funcLit := &FunctionLiteral{
		Token: methodToken,
	}

	// Expect '(' for parameters
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	// Parse parameters
	funcLit.Parameters, funcLit.RestParameter, _ = p.parseFunctionParameters(false)
	if funcLit.Parameters == nil && funcLit.RestParameter == nil {
		return nil // Error parsing parameters
	}

	// Check for optional return type annotation
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // Consume ')'
		p.nextToken() // Consume ':'
		funcLit.ReturnTypeAnnotation = p.parseTypeExpression()
		if funcLit.ReturnTypeAnnotation == nil {
			return nil // Error parsing return type
		}
	}

	// Expect '{' for method body
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	// Parse method body
	funcLit.Body = p.parseBlockStatement()
	if funcLit.Body == nil {
		return nil // Error parsing method body
	}

	// Transform function if it has destructuring parameters
	funcLit = p.transformFunctionWithDestructuring(funcLit)

	// Create MethodDefinition for object literal method
	method := &MethodDefinition{
		Token:       methodToken,
		Key:         methodName,
		Value:       funcLit,
		Kind:        "method",
		IsStatic:    false,
		IsPublic:    false,
		IsPrivate:   false,
		IsProtected: false,
		IsOverride:  false,
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

	// Check for type parameters (same pattern as functions)
	stmt.TypeParameters = p.tryParseTypeParameters()

	// Check for extends clause
	if p.peekTokenIs(lexer.EXTENDS) {
		p.nextToken() // Consume 'extends'

		// Parse list of extended interfaces (supporting generic types)
		for {
			p.nextToken() // Move to start of type expression

			// Parse full type expression (supports both simple identifiers and generic types)
			extendedType := p.parseTypeExpression()
			if extendedType == nil {
				return nil // Failed to parse extended interface type
			}

			stmt.Extends = append(stmt.Extends, extendedType)

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

	// Check for index signature or computed property: [...]
	if p.curTokenIs(lexer.LBRACKET) {
		return p.parseInterfaceBracketProperty()
	}

	// Check for shorthand method syntax first (identifier, keyword, or string literal as property name)
	propName := p.parsePropertyName()
	var prop *InterfaceProperty

	if propName != nil {
		prop = &InterfaceProperty{
			Name: propName,
		}
	} else if p.curTokenIs(lexer.STRING) {
		// Handle string literal property names
		stringLit := &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
		prop = &InterfaceProperty{
			ComputedName:       stringLit,
			IsComputedProperty: true,
		}
	} else {
		p.addError(p.curToken, "expected property name (identifier or string literal) or call signature '(' in interface")
		return nil
	}

	// Check for optional marker '?' first
	if p.peekTokenIs(lexer.QUESTION) {
		p.nextToken() // Consume '?'
		prop.Optional = true
	}

	// Check if this is a generic method signature: methodName<T, U>(...)
	var typeParams []*TypeParameter
	if p.peekTokenIs(lexer.LT) {
		p.nextToken() // Move to '<'
		var err error
		typeParams, err = p.parseTypeParameters()
		if err != nil {
			return nil // Error parsing type parameters
		}
	}

	// Check if this is a shorthand method signature
	if p.peekTokenIs(lexer.LPAREN) {
		// This is a shorthand method signature like methodName(): ReturnType or methodName<T>(): ReturnType
		p.nextToken() // Move to '('

		// Parse method type signature (uses ':' syntax, not '=>')
		funcType := p.parseMethodTypeSignature()
		if funcType == nil {
			return nil // Error parsing method type
		}

		// Add type parameters if this is a generic method
		if funcTypeExpr, ok := funcType.(*FunctionTypeExpression); ok && len(typeParams) > 0 {
			funcTypeExpr.TypeParameters = typeParams
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

// parseInterfaceBracketProperty parses both index signatures and computed property names
func (p *Parser) parseInterfaceBracketProperty() *InterfaceProperty {
	// We're currently at '[', move to the content
	p.nextToken()

	// Parse the expression inside the brackets
	expr := p.parseExpression(LOWEST)
	if expr == nil {
		return nil
	}

	// Look ahead to see if this is an index signature [key: type]: valueType
	// or a computed property [expr]: type
	if p.peekTokenIs(lexer.COLON) {
		// This is an index signature: [key: type]: valueType
		// The expression should be an identifier
		ident, ok := expr.(*Identifier)
		if !ok {
			p.addError(p.curToken, "index signature key must be an identifier")
			return nil
		}

		// Continue parsing as index signature
		prop := &InterfaceProperty{
			IsIndexSignature: true,
			KeyName:          ident,
		}

		// Expect ':'
		if !p.expectPeek(lexer.COLON) {
			return nil
		}

		// Parse key type
		p.nextToken() // Move to the start of the key type expression
		prop.KeyType = p.parseTypeExpression()
		if prop.KeyType == nil {
			return nil
		}

		// Expect ']'
		if !p.expectPeek(lexer.RBRACKET) {
			return nil
		}

		// Expect ':'
		if !p.expectPeek(lexer.COLON) {
			return nil
		}

		// Parse value type
		p.nextToken() // Move to the start of the value type expression
		prop.ValueType = p.parseTypeExpression()
		if prop.ValueType == nil {
			return nil
		}

		return prop
	} else if p.peekTokenIs(lexer.RBRACKET) {
		// This is a computed property: [expr]: type or [expr](...): type
		prop := &InterfaceProperty{
			IsComputedProperty: true,
			ComputedName:       expr,
		}

		// Expect ']'
		if !p.expectPeek(lexer.RBRACKET) {
			return nil
		}

		// Check for optional marker '?'
		if p.peekTokenIs(lexer.QUESTION) {
			p.nextToken() // Consume '?'
			prop.Optional = true
		}

		// Check if this is a shorthand method signature like [expr](...): ReturnType
		// or a generic method signature like [expr]<T>(...): ReturnType
		if p.peekTokenIs(lexer.LPAREN) {
			// This is a shorthand method signature
			p.nextToken() // Move to '('

			// Parse method type signature (uses ':' syntax, not '=>')
			funcType := p.parseMethodTypeSignature()
			if funcType == nil {
				return nil // Error parsing method type
			}

			prop.Type = funcType
			prop.IsMethod = true
			return prop
		} else if p.peekTokenIs(lexer.LT) {
			// This is a generic method signature like [expr]<T>(...): ReturnType
			p.nextToken() // Move to '<'

			// Parse type parameters
			typeParams, err := p.parseTypeParameters()
			if err != nil {
				p.addError(p.curToken, fmt.Sprintf("failed to parse type parameters: %v", err))
				return nil
			}

			// Expect '(' for parameters
			if !p.expectPeek(lexer.LPAREN) {
				return nil
			}

			// Parse method type signature (uses ':' syntax, not '=>')
			funcType := p.parseMethodTypeSignature()
			if funcType == nil {
				return nil // Error parsing method type
			}

			// Add type parameters to the function type
			if methodType, ok := funcType.(*FunctionTypeExpression); ok {
				methodType.TypeParameters = typeParams
			}

			prop.Type = funcType
			prop.IsMethod = true
			return prop
		}

		// Otherwise, expect ':' for property type
		if !p.expectPeek(lexer.COLON) {
			return nil
		}

		// Parse property type
		p.nextToken() // Move to the start of the type expression
		prop.Type = p.parseTypeExpression()
		if prop.Type == nil {
			return nil
		}

		return prop
	} else {
		p.addError(p.peekToken, "expected ':' or ']' after bracket expression in interface")
		return nil
	}
}

// parseInterfaceIndexSignature parses an interface index signature like [key: string]: Type
func (p *Parser) parseInterfaceIndexSignature() *InterfaceProperty {
	prop := &InterfaceProperty{
		IsIndexSignature: true,
	}

	// Expect identifier for key name
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	prop.KeyName = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Expect ':'
	if !p.expectPeek(lexer.COLON) {
		return nil
	}

	// Parse key type
	p.nextToken() // Move to the start of the key type expression
	prop.KeyType = p.parseTypeExpression()
	if prop.KeyType == nil {
		return nil
	}

	// Expect ']'
	if !p.expectPeek(lexer.RBRACKET) {
		return nil
	}

	// Expect ':'
	if !p.expectPeek(lexer.COLON) {
		return nil
	}

	// Parse value type
	p.nextToken() // Move to the start of the value type expression
	prop.ValueType = p.parseTypeExpression()
	if prop.ValueType == nil {
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

// parseObjectTypeExpression parses object type literals like { name: string; age: number }
// and mapped types like { [P in K]: T }.
func (p *Parser) parseObjectTypeExpression() Expression {
	startToken := p.curToken // The '{' token

	// Handle empty object type {}
	if p.peekTokenIs(lexer.RBRACE) {
		p.nextToken() // Consume '}'
		return &ObjectTypeExpression{
			Token:      startToken,
			Properties: []*ObjectTypeProperty{},
		}
	}

	// Check if this might be a mapped type by looking ahead
	// Pattern: { [readonly] [P in K][?: T }
	debugPrint("Checking if mapped type: cur=%s, peek=%s", p.curToken.Literal, p.peekToken.Literal)
	if p.peekTokenIs(lexer.LBRACKET) ||
		(p.peekTokenIs(lexer.READONLY) && p.peekTokenIs2(lexer.LBRACKET)) ||
		(p.peekTokenIs(lexer.MINUS) && p.peekTokenIs2(lexer.READONLY)) ||
		(p.peekTokenIs(lexer.PLUS) && p.peekTokenIs2(lexer.READONLY)) {

		debugPrint("Potential mapped type detected, checking pattern...")
		// Look ahead to check for 'in' keyword to distinguish mapped types from index signatures
		if isMappedType := p.isMappedTypePattern(); isMappedType {
			debugPrint("MAPPED TYPE DETECTED! Calling parseMappedTypeExpression")
			return p.parseMappedTypeExpression(startToken)
		} else {
			debugPrint("Not a mapped type pattern, continuing with object type")
		}
	} else {
		debugPrint("No bracket found, not a mapped type")
	}

	// Parse regular object type
	objType := &ObjectTypeExpression{
		Token:      startToken,
		Properties: []*ObjectTypeProperty{},
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
		} else if p.curTokenIs(lexer.LBRACKET) {
			// Handle both index signatures and computed properties
			prop := p.parseObjectTypeBracketProperty()
			if prop == nil {
				return nil
			}
			objType.Properties = append(objType.Properties, prop)
		} else {
			// Regular property or method signature - try to parse property name (allowing keywords and string literals)
			propName := p.parsePropertyName()
			var prop *ObjectTypeProperty

			if propName != nil {
				prop = &ObjectTypeProperty{
					Name: propName,
				}
			} else if p.curTokenIs(lexer.STRING) {
				// Handle string literal property names by creating an identifier with the string value
				stringLit := &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
				// Create an identifier from the string literal for compatibility
				identFromString := &Identifier{Token: p.curToken, Value: stringLit.Value}
				prop = &ObjectTypeProperty{
					Name: identFromString,
				}
			} else {
				p.addError(p.curToken, "expected property name (identifier or string literal), call signature '(', or index signature '[' in object type")
				return nil
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

		// Expect ';', ',' or '}' next
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken() // Consume ';'
		} else if p.peekTokenIs(lexer.COMMA) {
			p.nextToken() // Consume ','
		} else if p.peekTokenIs(lexer.RBRACE) {
			// End of object type, will be consumed by outer loop condition
			break
		} else {
			p.addError(p.peekToken, "expected ';', ',' or '}' after object type property")
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
	case lexer.PRIVATE_IDENT:
		// Support private field access: obj.#field
		return &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	case lexer.DELETE, lexer.GET, lexer.SET, lexer.IF, lexer.ELSE, lexer.FOR, lexer.WHILE, lexer.FUNCTION,
		lexer.RETURN, lexer.THROW, lexer.LET, lexer.CONST, lexer.TRUE, lexer.FALSE, lexer.NULL,
		lexer.UNDEFINED, lexer.THIS, lexer.NEW, lexer.TYPEOF, lexer.VOID, lexer.AS, lexer.SATISFIES,
		lexer.IN, lexer.INSTANCEOF, lexer.DO, lexer.ENUM, lexer.FROM, lexer.CATCH, lexer.FINALLY,
		lexer.TRY, lexer.SWITCH, lexer.CASE, lexer.DEFAULT, lexer.BREAK, lexer.CONTINUE, lexer.CLASS,
		lexer.STATIC, lexer.READONLY, lexer.PUBLIC, lexer.PRIVATE, lexer.PROTECTED, lexer.ABSTRACT,
		lexer.OVERRIDE, lexer.IMPORT, lexer.EXPORT, lexer.YIELD, lexer.AWAIT, lexer.VAR, lexer.TYPE, lexer.KEYOF,
		lexer.INFER, lexer.IS, lexer.OF, lexer.INTERFACE, lexer.EXTENDS, lexer.IMPLEMENTS, lexer.SUPER, lexer.WITH:
		// Allow keywords as property names
		return &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	default:
		return nil
	}
}

// parsePrivateIdent parses a standalone private identifier for use in 'in' expressions.
// Syntax: #field in obj - checks if private field exists on object
func (p *Parser) parsePrivateIdent() Expression {
	return &PrivateIdentifier{
		Token: p.curToken,
		Value: p.curToken.Literal, // includes the # prefix
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

// parseOptionalChainingExpression handles optional chaining expressions (obj?.prop, obj?.[expr], func?.())
func (p *Parser) parseOptionalChainingExpression(left Expression) Expression {
	// Current token should be OPTIONAL_CHAINING (?.)
	optToken := p.curToken // The '?.' token

	// Move to the next token to see what follows the ?.
	p.nextToken()

	switch p.curToken.Type {
	case lexer.LBRACKET:
		// Optional computed access: obj?.[expr]
		return p.parseOptionalIndexExpression(left, optToken)

	case lexer.LPAREN:
		// Optional call: func?.()
		return p.parseOptionalCallExpression(left, optToken)

	default:
		// Optional property access: obj?.prop (current behavior)
		exp := &OptionalChainingExpression{
			Token:  optToken,
			Object: left,
		}

		// Parse property name (allowing keywords as property names)
		propIdent := p.parsePropertyName()
		if propIdent == nil {
			// If the token after '?.' is not a valid property name, it's a syntax error.
			msg := fmt.Sprintf("expected identifier after '?.', got %s", p.curToken.Type)
			p.addError(p.curToken, msg)
			return nil
		}

		exp.Property = propIdent
		return exp
	}
}

// parseOptionalIndexExpression handles optional computed access (obj?.[expr])
func (p *Parser) parseOptionalIndexExpression(left Expression, optToken lexer.Token) Expression {
	exp := &OptionalIndexExpression{
		Token:  optToken, // The '?.' token
		Object: left,
	}

	// Current token is already '[', move past it to the index expression
	p.nextToken()
	exp.Index = p.parseExpression(LOWEST)
	if exp.Index == nil {
		return nil
	}

	// Expect closing bracket
	if !p.expectPeek(lexer.RBRACKET) {
		return nil
	}

	return exp
}

// parseOptionalCallExpression handles optional function calls (func?.())
func (p *Parser) parseOptionalCallExpression(left Expression, optToken lexer.Token) Expression {
	exp := &OptionalCallExpression{
		Token:    optToken, // The '?.' token
		Function: left,
	}

	// Current token is '(', parse the arguments
	exp.Arguments = p.parseExpressionList(lexer.RPAREN)
	if exp.Arguments == nil {
		return nil
	}

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
func (p *Parser) parseForOfStatement() *ForOfStatement {
	stmt := &ForOfStatement{Token: p.curToken} // 'for'

	if !p.expectPeek(lexer.LPAREN) { // Consume '(', cur='('
		return nil
	}

	// Parse variable declaration or identifier
	p.nextToken() // Move past '('
	debugPrint("parseForOfStatement: Variable START, cur='%s'", p.curToken.Literal)

	if p.curTokenIs(lexer.LET) {
		// Parse let declaration
		letStmt := &LetStatement{Token: p.curToken}
		if !p.expectPeek(lexer.IDENT) {
			return nil
		}
		declarator := &VarDeclarator{}
		declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		letStmt.Declarations = []*VarDeclarator{declarator}
		letStmt.Name = declarator.Name
		// Note: No type annotation or value assignment in for...of
		stmt.Variable = letStmt
	} else if p.curTokenIs(lexer.CONST) {
		// Parse const declaration
		constStmt := &ConstStatement{Token: p.curToken}
		if !p.expectPeek(lexer.IDENT) {
			return nil
		}
		declarator := &VarDeclarator{}
		declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		constStmt.Declarations = []*VarDeclarator{declarator}
		constStmt.Name = declarator.Name
		stmt.Variable = constStmt
	} else if p.curTokenIs(lexer.VAR) {
		// Parse var declaration
		varStmt := &VarStatement{Token: p.curToken}
		if !p.expectPeek(lexer.IDENT) {
			return nil
		}
		declarator := &VarDeclarator{}
		declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		varStmt.Declarations = []*VarDeclarator{declarator}
		varStmt.Name = declarator.Name
		stmt.Variable = varStmt
	} else if p.curTokenIs(lexer.IDENT) {
		// Parse bare identifier (reusing existing variable)
		ident := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
		exprStmt := &ExpressionStatement{Token: p.curToken, Expression: ident}
		stmt.Variable = exprStmt
	} else {
		return nil
	}

	// Expect 'of'
	if !p.expectPeek(lexer.OF) {
		return nil
	}
	debugPrint("parseForOfStatement: Found 'of', cur='%s'", p.curToken.Literal)

	// Parse iterable expression
	p.nextToken() // Move past 'of'
	debugPrint("parseForOfStatement: Parsing iterable, cur='%s'", p.curToken.Literal)
	stmt.Iterable = p.parseExpression(LOWEST)

	// Expect ')'
	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}
	debugPrint("parseForOfStatement: Found ')', cur='%s'", p.curToken.Literal)

	// Parse body (same logic as regular for loop)
	stmt.Body = p.parseForBody()

	debugPrint("parseForOfStatement: FINISHED")

	return stmt
}

// parseForStatementOrForOf determines if this is for...of, for...in, or regular for and parses accordingly
func (p *Parser) parseForStatementOrForOf(forToken lexer.Token, isAsync bool) Statement {
	// We're positioned at the variable declaration or identifier
	// Parse the variable part and see what comes next

	var varStmt Statement
	var varName string

	if p.curTokenIs(lexer.LET) {
		letToken := p.curToken
		p.nextToken() // Move past LET

		// Check for destructuring patterns
		if p.curTokenIs(lexer.LBRACKET) {
			// Array destructuring: for(let [a, b] ...)
			// Parse pattern without requiring initializer initially
			varStmt = p.parseArrayDestructuringDeclaration(letToken, false, false)
			varName = "" // Destructuring doesn't have a single name

			// If parsing failed (e.g., invalid syntax), varStmt will be a typed nil
			// In Go, when a function returns a typed nil pointer (*T)(nil) and it's assigned
			// to an interface variable, the interface is not nil even though the value is nil
			if varStmt == nil || varStmt.(*ArrayDestructuringDeclaration) == nil {
				return nil
			}

			// Check if there's an initializer for regular for loops: for(let [a,b] = [1,2]; ...)
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken() // consume '='
				p.nextToken() // move to RHS
				if arrayDecl, ok := varStmt.(*ArrayDestructuringDeclaration); ok {
					arrayDecl.Value = p.parseExpression(LOWEST)
				}
			}
		} else if p.curTokenIs(lexer.LBRACE) {
			// Object destructuring: for(let {a, b} ...)
			varStmt = p.parseObjectDestructuringDeclaration(letToken, false, false)
			varName = "" // Destructuring doesn't have a single name

			// If parsing failed (e.g., invalid syntax), varStmt will be a typed nil
			// In Go, when a function returns a typed nil pointer (*T)(nil) and it's assigned
			// to an interface variable, the interface is not nil even though the value is nil
			if varStmt == nil || varStmt.(*ObjectDestructuringDeclaration) == nil {
				return nil
			}

			// Check if there's an initializer for regular for loops: for(let {a,b} = obj; ...)
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken() // consume '='
				p.nextToken() // move to RHS
				if objDecl, ok := varStmt.(*ObjectDestructuringDeclaration); ok {
					objDecl.Value = p.parseExpression(LOWEST)
				}
			}
		} else if p.curTokenIs(lexer.IDENT) {
			// Regular identifier
			letStmt := &LetStatement{Token: letToken}
			declarator := &VarDeclarator{}
			declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
			letStmt.Declarations = []*VarDeclarator{declarator}
			letStmt.Name = declarator.Name
			varStmt = letStmt
			varName = p.curToken.Literal
		} else {
			p.addError(p.curToken, fmt.Sprintf("expected identifier or destructuring pattern after 'let', got %s", p.curToken.Type))
			return nil
		}
	} else if p.curTokenIs(lexer.CONST) {
		constToken := p.curToken
		p.nextToken() // Move past CONST

		// Check for destructuring patterns
		if p.curTokenIs(lexer.LBRACKET) {
			// Array destructuring: for(const [a, b] ...)
			varStmt = p.parseArrayDestructuringDeclaration(constToken, true, false)
			varName = "" // Destructuring doesn't have a single name

			// If parsing failed (e.g., invalid syntax), varStmt will be a typed nil
			// In Go, when a function returns a typed nil pointer (*T)(nil) and it's assigned
			// to an interface variable, the interface is not nil even though the value is nil
			if varStmt == nil || varStmt.(*ArrayDestructuringDeclaration) == nil {
				return nil
			}

			// Check if there's an initializer for regular for loops
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken() // consume '='
				p.nextToken() // move to RHS
				if arrayDecl, ok := varStmt.(*ArrayDestructuringDeclaration); ok {
					arrayDecl.Value = p.parseExpression(LOWEST)
				}
			}
		} else if p.curTokenIs(lexer.LBRACE) {
			// Object destructuring: for(const {a, b} ...)
			varStmt = p.parseObjectDestructuringDeclaration(constToken, true, false)
			varName = "" // Destructuring doesn't have a single name

			// If parsing failed (e.g., invalid syntax), varStmt will be a typed nil
			// In Go, when a function returns a typed nil pointer (*T)(nil) and it's assigned
			// to an interface variable, the interface is not nil even though the value is nil
			if varStmt == nil || varStmt.(*ObjectDestructuringDeclaration) == nil {
				return nil
			}

			// Check if there's an initializer for regular for loops
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken() // consume '='
				p.nextToken() // move to RHS
				if objDecl, ok := varStmt.(*ObjectDestructuringDeclaration); ok {
					objDecl.Value = p.parseExpression(LOWEST)
				}
			}
		} else if p.curTokenIs(lexer.IDENT) {
			// Regular identifier
			constStmt := &ConstStatement{Token: constToken}
			declarator := &VarDeclarator{}
			declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
			constStmt.Declarations = []*VarDeclarator{declarator}
			constStmt.Name = declarator.Name
			varStmt = constStmt
			varName = p.curToken.Literal
		} else {
			p.addError(p.curToken, fmt.Sprintf("expected identifier or destructuring pattern after 'const', got %s", p.curToken.Type))
			return nil
		}
	} else if p.curTokenIs(lexer.VAR) {
		varToken := p.curToken
		p.nextToken() // Move past VAR

		// Check for destructuring patterns
		if p.curTokenIs(lexer.LBRACKET) {
			// Array destructuring: for(var [a, b] ...)
			varStmt = p.parseArrayDestructuringDeclaration(varToken, false, false)
			varName = "" // Destructuring doesn't have a single name

			// Check if there's an initializer for regular for loops
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken() // consume '='
				p.nextToken() // move to RHS
				if arrayDecl, ok := varStmt.(*ArrayDestructuringDeclaration); ok && arrayDecl != nil {
					arrayDecl.Value = p.parseExpression(LOWEST)
				}
			}
		} else if p.curTokenIs(lexer.LBRACE) {
			// Object destructuring: for(var {a, b} ...)
			varStmt = p.parseObjectDestructuringDeclaration(varToken, false, false)
			varName = "" // Destructuring doesn't have a single name

			// Check if there's an initializer for regular for loops
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken() // consume '='
				p.nextToken() // move to RHS
				if objDecl, ok := varStmt.(*ObjectDestructuringDeclaration); ok && objDecl != nil {
					objDecl.Value = p.parseExpression(LOWEST)
				}
			}
		} else if p.curTokenIs(lexer.IDENT) {
			// Regular identifier
			varDeclaration := &VarStatement{Token: varToken}
			declarator := &VarDeclarator{}
			declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
			varDeclaration.Declarations = []*VarDeclarator{declarator}
			varDeclaration.Name = declarator.Name
			varStmt = varDeclaration
			varName = p.curToken.Literal
		} else {
			p.addError(p.curToken, fmt.Sprintf("expected identifier or destructuring pattern after 'var', got %s", p.curToken.Type))
			return nil
		}
	} else if p.curTokenIs(lexer.IDENT) {
		// Could be bare identifier or member expression
		// Check if followed by . or [ to determine if it's a member expression
		if p.peekTokenIs(lexer.DOT) || p.peekTokenIs(lexer.LBRACKET) {
			// Member expression: parse it fully
			expr := p.parseExpression(LOWEST)
			exprStmt := &ExpressionStatement{Token: p.curToken, Expression: expr}
			varStmt = exprStmt
			varName = "" // Member expression doesn't have a simple name
		} else {
			// Simple identifier
			ident := &Identifier{Token: p.curToken, Value: p.curToken.Literal}
			exprStmt := &ExpressionStatement{Token: p.curToken, Expression: ident}
			varStmt = exprStmt
			varName = p.curToken.Literal
		}
	} else if p.curTokenIs(lexer.LBRACKET) {
		// Array destructuring assignment: for ([x] of items)
		// Parse as ArrayLiteral (which acts as assignment pattern)
		arrayPattern := p.parseArrayLiteral()
		exprStmt := &ExpressionStatement{Token: p.curToken, Expression: arrayPattern}
		varStmt = exprStmt
		varName = "" // Destructuring doesn't have a single name
	} else if p.curTokenIs(lexer.LBRACE) {
		// Object destructuring assignment: for ({a} of items)
		// Parse as ObjectLiteral (which acts as assignment pattern)
		objPattern := p.parseObjectLiteral()
		exprStmt := &ExpressionStatement{Token: p.curToken, Expression: objPattern}
		varStmt = exprStmt
		varName = "" // Destructuring doesn't have a single name
	} else {
		return nil
	}

	// Check what comes after the variable
	if p.peekTokenIs(lexer.OF) {
		// This is a for...of loop!
		p.nextToken() // consume variable name, cur='of'

		stmt := &ForOfStatement{Token: forToken, IsAsync: isAsync}
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
		if isAsync {
			p.addError(p.curToken, "for-await can only be used with for-of loops, not for-in loops")
			return nil
		}
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
		// for-await cannot be used with regular for loops
		if isAsync {
			p.addError(p.curToken, "for-await can only be used with for-of loops, not regular for loops")
			return nil
		}
		// We need to continue parsing as regular for loop
		// Reset and parse as regular for statement
		return p.parseRegularForStatementWithVar(forToken, varStmt, varName)
	}
}

// parseRegularForStatement parses a standard C-style for loop
// Precondition: curToken is the first token after '(' or we're at '(' for empty initializer
func (p *Parser) parseRegularForStatement(forToken lexer.Token) *ForStatement {
	stmt := &ForStatement{Token: forToken}

	debugPrint("parseRegularForStatement: START")

	// --- 1. Parse Initializer ---
	// Check if we need to advance (for empty initializer case)
	if p.curTokenIs(lexer.LPAREN) {
		// We're still at '(', need to advance
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken() // Move to ';'
			stmt.Initializer = nil
		} else {
			p.nextToken() // Move to start of initializer
		}
	}

	// Now parse initializer if curToken is not SEMICOLON
	if !p.curTokenIs(lexer.SEMICOLON) {
		if p.curTokenIs(lexer.LET) {
			letStmt := &LetStatement{Token: p.curToken}
			if !p.expectPeek(lexer.IDENT) {
				return nil
			}
			declarator := &VarDeclarator{}
			declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken()
				p.nextToken()
				declarator.TypeAnnotation = p.parseTypeExpression()
			}
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken()
				p.nextToken()
				// Use COMMA precedence to allow assignment expressions but stop at commas
				declarator.Value = p.parseExpression(COMMA)
			}
			letStmt.Declarations = []*VarDeclarator{declarator}
			// Set legacy fields for backward compatibility
			letStmt.Name = declarator.Name
			letStmt.TypeAnnotation = declarator.TypeAnnotation
			letStmt.Value = declarator.Value
			stmt.Initializer = letStmt
		} else if p.curTokenIs(lexer.CONST) {
			constStmt := &ConstStatement{Token: p.curToken}
			if !p.expectPeek(lexer.IDENT) {
				return nil
			}
			declarator := &VarDeclarator{}
			declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken()
				p.nextToken()
				declarator.TypeAnnotation = p.parseTypeExpression()
			}
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken()
				p.nextToken()
				// Use COMMA precedence to allow assignment expressions but stop at commas
				declarator.Value = p.parseExpression(COMMA)
			}
			constStmt.Declarations = []*VarDeclarator{declarator}
			constStmt.Name = declarator.Name
			constStmt.TypeAnnotation = declarator.TypeAnnotation
			constStmt.Value = declarator.Value
			stmt.Initializer = constStmt
		} else if p.curTokenIs(lexer.VAR) {
			varStmt := &VarStatement{Token: p.curToken}
			if !p.expectPeek(lexer.IDENT) {
				return nil
			}
			declarator := &VarDeclarator{}
			declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken()
				p.nextToken()
				declarator.TypeAnnotation = p.parseTypeExpression()
			}
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken()
				p.nextToken()
				// Use COMMA precedence to allow assignment expressions but stop at commas
				declarator.Value = p.parseExpression(COMMA)
			}
			varStmt.Declarations = []*VarDeclarator{declarator}
			varStmt.Name = declarator.Name
			varStmt.TypeAnnotation = declarator.TypeAnnotation
			varStmt.Value = declarator.Value
			stmt.Initializer = varStmt
		} else {
			// Expression initializer (handles any expression including function calls)
			exprStmt := &ExpressionStatement{Token: p.curToken}
			exprStmt.Expression = p.parseExpression(LOWEST)
			stmt.Initializer = exprStmt
		}
		if !p.expectPeek(lexer.SEMICOLON) {
			return nil
		}
	} else {
		// Empty initializer, curToken is already at ';'
		// Don't advance yet - let condition parsing handle it
	}

	// --- 2. Parse Condition ---
	// At this point, curToken is ';' (after initializer)
	// Advance past it to get to condition start (or second ';' if empty condition)
	p.nextToken()

	if !p.curTokenIs(lexer.SEMICOLON) {
		stmt.Condition = p.parseExpression(LOWEST)
		if !p.expectPeek(lexer.SEMICOLON) {
			return nil
		}
	} else {
		// Empty condition, curToken is already at second ';'
		// Don't advance yet - let update parsing handle it
		stmt.Condition = nil
	}

	// --- 3. Parse Update ---
	// At this point, curToken is ';' (after condition)
	// Advance past it to get to update start (or ')' if empty update)
	p.nextToken()

	if !p.curTokenIs(lexer.RPAREN) {
		stmt.Update = p.parseExpression(LOWEST)
		if !p.expectPeek(lexer.RPAREN) {
			return nil
		}
	} else {
		// Empty update, curToken is already at ')'
		// Don't advance yet - let body parsing handle it
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
		// Handle type annotation for var statements as in TypeScript
		if vs, ok := varStmt.(*VarStatement); ok {
			p.nextToken()
			p.nextToken()
			vs.TypeAnnotation = p.parseTypeExpression()
		}
	}
	if p.peekTokenIs(lexer.ASSIGN) {
		// Handle assignment
		p.nextToken()
		p.nextToken()
		if letStmt, ok := varStmt.(*LetStatement); ok {
			// Use COMMA precedence to stop at comma separators for multi-variable declarations
			letStmt.Value = p.parseExpression(COMMA)
		} else if constStmt, ok := varStmt.(*ConstStatement); ok {
			// Use COMMA precedence to stop at comma separators for multi-variable declarations
			constStmt.Value = p.parseExpression(COMMA)
		} else if vs, ok := varStmt.(*VarStatement); ok {
			// Use COMMA precedence to stop at comma separators for multi-variable declarations
			vs.Value = p.parseExpression(COMMA)
		}
		// For expression statements, we'd need to create an assignment expression
	}

	// Sync Declarations with legacy fields for all statement types
	if vs, ok := varStmt.(*VarStatement); ok {
		if vs.Declarations != nil && len(vs.Declarations) > 0 {
			vs.Declarations[0].Value = vs.Value
			vs.Declarations[0].TypeAnnotation = vs.TypeAnnotation
		}
		// Parse additional comma-separated declarations: for (var x = 1, y = 2; ...)
		for p.peekTokenIs(lexer.COMMA) {
			p.nextToken() // Consume ','
			if !p.expectPeekIdentifierOrKeyword() {
				return nil
			}
			declarator := &VarDeclarator{}
			declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
			// Optional Type Annotation
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken() // Consume ':'
				p.nextToken() // Consume token starting the type expression
				declarator.TypeAnnotation = p.parseTypeExpression()
			}
			// Optional assignment
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken() // Consume '='
				p.nextToken() // Consume token starting the expression
				declarator.Value = p.parseExpression(COMMA)
			}
			vs.Declarations = append(vs.Declarations, declarator)
		}
	} else if letStmt, ok := varStmt.(*LetStatement); ok {
		if letStmt.Declarations != nil && len(letStmt.Declarations) > 0 {
			letStmt.Declarations[0].Value = letStmt.Value
			letStmt.Declarations[0].TypeAnnotation = letStmt.TypeAnnotation
		}
		// Parse additional comma-separated declarations: for (let x = 1, y = 2; ...)
		for p.peekTokenIs(lexer.COMMA) {
			p.nextToken() // Consume ','
			if !p.expectPeekIdentifierOrKeyword() {
				return nil
			}
			declarator := &VarDeclarator{}
			declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
			// Optional Type Annotation
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken() // Consume ':'
				p.nextToken() // Consume token starting the type expression
				declarator.TypeAnnotation = p.parseTypeExpression()
			}
			// Optional assignment
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken() // Consume '='
				p.nextToken() // Consume token starting the expression
				declarator.Value = p.parseExpression(COMMA)
			}
			letStmt.Declarations = append(letStmt.Declarations, declarator)
		}
	} else if constStmt, ok := varStmt.(*ConstStatement); ok {
		if constStmt.Declarations != nil && len(constStmt.Declarations) > 0 {
			constStmt.Declarations[0].Value = constStmt.Value
			constStmt.Declarations[0].TypeAnnotation = constStmt.TypeAnnotation
		}
		// Parse additional comma-separated declarations: for (const x = 1, y = 2; ...)
		for p.peekTokenIs(lexer.COMMA) {
			p.nextToken() // Consume ','
			if !p.expectPeekIdentifierOrKeyword() {
				return nil
			}
			declarator := &VarDeclarator{}
			declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
			// Optional Type Annotation
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken() // Consume ':'
				p.nextToken() // Consume token starting the type expression
				declarator.TypeAnnotation = p.parseTypeExpression()
			}
			// Optional assignment (const requires it, but parser doesn't enforce)
			if p.peekTokenIs(lexer.ASSIGN) {
				p.nextToken() // Consume '='
				p.nextToken() // Consume token starting the expression
				declarator.Value = p.parseExpression(COMMA)
			}
			constStmt.Declarations = append(constStmt.Declarations, declarator)
		}
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

		// Handle optional parameter name with potential '?' token
		if p.curTokenIs(lexer.IDENT) {
			if p.peekTokenIs(lexer.QUESTION) {
				// Optional parameter: name?: type
				p.nextToken() // Consume IDENT
				p.nextToken() // Consume '?'
				// Current token should now be ':', just advance to the type
				if !p.curTokenIs(lexer.COLON) {
					p.addError(p.curToken, "expected ':' after '?' in optional parameter")
					return nil
				}
				p.nextToken() // Move to the actual type
			} else if p.peekTokenIs(lexer.COLON) {
				// Required parameter: name: type
				p.nextToken() // Consume IDENT
				p.nextToken() // Consume ':', move to the actual type
			}
			// else: just a type without parameter name
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

			// Handle trailing comma - if we see ')' after a comma, we're done
			if p.curTokenIs(lexer.RPAREN) {
				// This is a trailing comma, we're already at the closing paren
				// Don't need to expectPeek for RPAREN later
				break
			}

			// Handle optional parameter name with potential '?' token
			if p.curTokenIs(lexer.IDENT) {
				if p.peekTokenIs(lexer.QUESTION) {
					// Optional parameter: name?: type
					p.nextToken() // Consume IDENT
					p.nextToken() // Consume '?'
					// Current token should now be ':', just advance to the type
					if !p.curTokenIs(lexer.COLON) {
						p.addError(p.curToken, "expected ':' after '?' in optional parameter")
						return nil
					}
					p.nextToken() // Move to the actual type
				} else if p.peekTokenIs(lexer.COLON) {
					// Required parameter: name: type
					p.nextToken() // Consume IDENT
					p.nextToken() // Consume ':', move to the actual type
				}
				// else: just a type without parameter name
			} // Now curToken should be the start of the type expression

			paramType := p.parseTypeExpression()
			if paramType == nil {
				return nil
			}
			params = append(params, paramType)
		}

		// Expect closing parenthesis (unless we already consumed it due to trailing comma)
		if !p.curTokenIs(lexer.RPAREN) && !p.expectPeek(lexer.RPAREN) {
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
	case *GenericTypeRef:
		return n.Token // The identifier token

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

// --- Type Parameter Parsing Helpers ---

// parseTypeParameters parses a type parameter list: <T, U extends string>
// Assumes current token is '<', consumes through '>'
func (p *Parser) parseTypeParameters() ([]*TypeParameter, error) {
	if !p.curTokenIs(lexer.LT) {
		return nil, fmt.Errorf("internal error: parseTypeParameters called without '<'")
	}

	var typeParams []*TypeParameter

	// Move to first type parameter
	p.nextToken()

	// Handle empty type parameter list
	if p.curTokenIs(lexer.GT) {
		return typeParams, nil // Empty list is valid
	}

	// Parse first type parameter
	firstParam := p.parseTypeParameter()
	if firstParam == nil {
		return nil, fmt.Errorf("failed to parse type parameter")
	}
	typeParams = append(typeParams, firstParam)

	// Parse remaining type parameters
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken() // consume comma

		// Check for trailing comma
		if p.peekTokenIs(lexer.GT) {
			break // Trailing comma before closing >
		}

		p.nextToken() // move to next type parameter

		param := p.parseTypeParameter()
		if param == nil {
			return nil, fmt.Errorf("failed to parse type parameter")
		}
		typeParams = append(typeParams, param)
	}

	// Expect closing '>'
	if !p.expectPeek(lexer.GT) {
		return nil, fmt.Errorf("expected '>' after type parameters")
	}

	return typeParams, nil
}

// parseTypeParameter parses a single type parameter: T or T extends string or T = DefaultType or T extends string = DefaultType
func (p *Parser) parseTypeParameter() *TypeParameter {
	if !p.curTokenIs(lexer.IDENT) {
		p.addError(p.curToken, "expected type parameter name")
		return nil
	}

	param := &TypeParameter{
		Token: p.curToken,
		Name:  &Identifier{Token: p.curToken, Value: p.curToken.Literal},
	}

	// Check for constraint: T extends SomeType
	if p.peekTokenIs(lexer.EXTENDS) {
		p.nextToken() // consume 'extends'
		p.nextToken() // move to constraint type

		constraint := p.parseTypeExpression()
		if constraint == nil {
			p.addError(p.curToken, "expected type after 'extends'")
			return nil
		}
		param.Constraint = constraint
	}

	// Check for default type: T = DefaultType (can be combined with constraint: T extends string = DefaultType)
	if p.peekTokenIs(lexer.ASSIGN) {
		p.nextToken() // consume '='
		p.nextToken() // move to default type

		defaultType := p.parseTypeExpression()
		if defaultType == nil {
			p.addError(p.curToken, "expected type after '='")
			return nil
		}

		param.DefaultType = defaultType
	}

	return param
}

// tryParseTypeParameters attempts to parse type parameters and returns them if successful
// If parsing fails, it backtracks and returns nil (used for disambiguation)
func (p *Parser) tryParseTypeParameters() []*TypeParameter {
	if !p.peekTokenIs(lexer.LT) {
		return nil // No type parameters
	}

	// Save current state for potential backtracking
	savedCur := p.curToken
	savedPeek := p.peekToken
	savedErrorCount := len(p.errors)

	// Try to parse type parameters
	p.nextToken() // consume '<'
	typeParams, err := p.parseTypeParameters()

	if err != nil {
		// Backtrack on failure
		p.curToken = savedCur
		p.peekToken = savedPeek
		// Remove any errors added during failed parse
		if len(p.errors) > savedErrorCount {
			p.errors = p.errors[:savedErrorCount]
		}
		return nil
	}

	return typeParams
}

// parseGenericArrowFunction parses arrow functions that start with type parameters
// Handles syntax like: <T>(x: T) => x or <T, U>(a: T, b: U) => [a, b]
func (p *Parser) parseGenericArrowFunction() Expression {
	if !p.curTokenIs(lexer.LT) {
		p.addError(p.curToken, "internal error: parseGenericArrowFunction called without '<'")
		return nil
	}

	// Parse type parameters
	typeParams, err := p.parseTypeParameters()
	if err != nil {
		p.addError(p.curToken, fmt.Sprintf("failed to parse type parameters: %v", err))
		return nil
	}

	// Expect '(' for parameters
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	// Parse regular parameters
	params, restParam, parseErr := p.parseFunctionParameters(false) // No parameter properties in function type parameter lists
	if parseErr != nil {
		p.addError(p.curToken, fmt.Sprintf("failed to parse arrow function parameters: %v", parseErr))
		return nil
	}

	// Optional return type annotation
	var returnTypeAnnotation Expression
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // consume ':'
		p.nextToken() // move to type
		returnTypeAnnotation = p.parseTypeExpression()
		if returnTypeAnnotation == nil {
			p.addError(p.curToken, "expected return type after ':'")
			return nil
		}
	}

	// Expect '=>'
	if !p.expectPeek(lexer.ARROW) {
		return nil
	}

	return p.parseArrowFunctionBodyAndFinish(typeParams, params, restParam, returnTypeAnnotation)
}

// parseGenericFunctionTypeExpression parses generic function types in type annotation context
// Handles syntax like: <T>(x: T) => T or <T, U>(a: T, b: U) => [T, U]
func (p *Parser) parseGenericFunctionTypeExpression() Expression {
	if !p.curTokenIs(lexer.LT) {
		p.addError(p.curToken, "internal error: parseGenericFunctionTypeExpression called without '<'")
		return nil
	}

	// Parse type parameters
	typeParams, err := p.parseTypeParameters()
	if err != nil {
		p.addError(p.curToken, fmt.Sprintf("failed to parse type parameters: %v", err))
		return nil
	}

	// Expect '(' for parameters
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	// Parse function type parameters (for type annotations)
	params, restParam, parseErr := p.parseFunctionTypeParameterList()
	if parseErr != nil {
		p.addError(p.curToken, fmt.Sprintf("failed to parse function type parameters: %v", parseErr))
		return nil
	}

	// Expect '=>' (no optional return type annotation in type context)
	if !p.expectPeek(lexer.ARROW) {
		return nil
	}

	// Parse return type
	p.nextToken() // move to return type
	returnType := p.parseTypeExpression()
	if returnType == nil {
		p.addError(p.curToken, "expected return type after '=>'")
		return nil
	}

	// Create a FunctionTypeExpression with generics
	funcType := &FunctionTypeExpression{
		Token:          p.curToken, // Should be the '(' token
		TypeParameters: typeParams,
		Parameters:     params,
		RestParameter:  restParam,
		ReturnType:     returnType,
	}

	return funcType
}

// isValidDestructuringTarget checks if an expression can be used as a destructuring target
// Valid targets: Identifier, ArrayLiteral (for nested array destructuring), ObjectLiteral (for nested object destructuring)
func (p *Parser) isValidDestructuringTarget(expr Expression) bool {
	switch expr.(type) {
	case *Identifier:
		// Simple variable assignment: [a] = [1]
		return true
	case *ArrayLiteral:
		// Nested array destructuring: [a, [b, c]] = [1, [2, 3]]
		return true
	case *ObjectLiteral:
		// Nested object destructuring: {user: {name, age}} = {user: {name: "John", age: 30}}
		return true
	case *UndefinedLiteral:
		// Elision in destructuring: [,] or [[,]]
		return true
	case *MemberExpression:
		// Member access: [obj.prop] = [1] or [obj[key]] = [1]
		return true
	case *IndexExpression:
		// Index access: [arr[0]] = [1]
		return true
	default:
		return false
	}
}

// GetSource returns the source file associated with this parser
func (p *Parser) GetSource() *source.SourceFile {
	return p.source
}

// --- Exception Handling Parsing ---

// parseTryStatement parses a try/catch/finally statement
func (p *Parser) parseTryStatement() *TryStatement {
	stmt := &TryStatement{Token: p.curToken} // 'try' token

	if !p.expectPeek(lexer.LBRACE) {
		return nil // Expected '{' after 'try'
	}

	stmt.Body = p.parseBlockStatement()
	if stmt.Body == nil {
		return nil
	}

	// Optional catch clause
	if p.peekTokenIs(lexer.CATCH) {
		p.nextToken() // consume 'catch'
		stmt.CatchClause = p.parseCatchClause()
		if stmt.CatchClause == nil {
			return nil
		}
	}

	// Optional finally clause (Phase 3)
	if p.peekTokenIs(lexer.FINALLY) {
		p.nextToken() // consume 'finally'

		if !p.expectPeek(lexer.LBRACE) {
			p.addError(p.curToken, "expected '{' after 'finally'")
			return nil
		}

		stmt.FinallyBlock = p.parseBlockStatement()
		if stmt.FinallyBlock == nil {
			return nil
		}
	}

	// Must have either catch or finally
	if stmt.CatchClause == nil && stmt.FinallyBlock == nil {
		p.addError(stmt.Token, "try statement must have a catch clause, finally clause, or both")
		return nil
	}

	return stmt
}

// parseCatchClause parses a catch clause
func (p *Parser) parseCatchClause() *CatchClause {
	clause := &CatchClause{Token: p.curToken} // 'catch' token

	// Optional parameter (ES2019+ allows catch without parameter)
	if p.peekTokenIs(lexer.LPAREN) {
		p.nextToken() // consume '('
		p.nextToken() // move to parameter

		// Parameter can be identifier or destructuring pattern
		// Note: await/yield can be identifiers in non-async/non-generator contexts
		switch p.curToken.Type {
		case lexer.IDENT:
			clause.Parameter = &Identifier{
				Token: p.curToken,
				Value: p.curToken.Literal,
			}
		case lexer.AWAIT:
			// await is valid as catch parameter in non-async context
			if p.inAsyncFunction == 0 {
				clause.Parameter = &Identifier{
					Token: p.curToken,
					Value: p.curToken.Literal,
				}
			} else {
				p.addError(p.curToken, "await is not allowed as catch parameter in async function")
				return nil
			}
		case lexer.YIELD:
			// yield is valid as catch parameter in non-generator context
			if p.inGenerator == 0 {
				clause.Parameter = &Identifier{
					Token: p.curToken,
					Value: p.curToken.Literal,
				}
			} else {
				p.addError(p.curToken, "yield is not allowed as catch parameter in generator function")
				return nil
			}
		case lexer.LBRACKET:
			// Array destructuring: catch ([x, y])
			clause.Parameter = p.parseArrayParameterPattern()
		case lexer.LBRACE:
			// Object destructuring: catch ({message, code})
			clause.Parameter = p.parseObjectParameterPattern()
		default:
			p.addError(p.curToken, fmt.Sprintf("expected identifier or destructuring pattern for catch parameter, got %s", p.curToken.Type))
			return nil
		}

		if !p.expectPeek(lexer.RPAREN) {
			return nil // Expected ')' after catch parameter
		}
	}

	if !p.expectPeek(lexer.LBRACE) {
		return nil // Expected '{' for catch body
	}

	clause.Body = p.parseBlockStatement()
	if clause.Body == nil {
		return nil
	}

	return clause
}

// parseThrowStatement parses a throw statement
func (p *Parser) parseThrowStatement() *ThrowStatement {
	stmt := &ThrowStatement{Token: p.curToken} // 'throw' token
	throwLine := p.curToken.Line

	// In JavaScript, throw requires an expression on the same line
	if p.peekTokenIs(lexer.SEMICOLON) || p.peekTokenIs(lexer.EOF) {
		p.addError(p.curToken, "throw statement requires an expression")
		return nil
	}

	p.nextToken() // move to expression

	// ASI: If there's a line terminator after 'throw', it's a syntax error
	// This is a restricted production - no LineTerminator allowed after throw
	if p.curToken.Line != throwLine {
		p.addError(stmt.Token, "Illegal newline after throw")
		return nil
	}

	stmt.Value = p.parseExpression(LOWEST)
	if stmt.Value == nil {
		return nil
	}

	// Optional semicolon
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) tryParseTypeArguments() []Expression {
	// Check if we're currently at '<' or if peek is '<'
	if !p.curTokenIs(lexer.LT) && !p.peekTokenIs(lexer.LT) {
		return nil // No type arguments
	}

	// Save current state for potential backtracking (tokens and lexer)
	savedCur := p.curToken
	savedPeek := p.peekToken
	savedErrorCount := len(p.errors)
	lexerState := p.l.SaveState()

	// If we're not at '<', advance to it
	if !p.curTokenIs(lexer.LT) {
		p.nextToken() // consume current token to get to '<'
	}

	typeArgs, err := p.parseTypeArguments()

	if err != nil {
		// Backtrack on failure
		p.l.RestoreState(lexerState)
		p.curToken = savedCur
		p.peekToken = savedPeek
		// Remove any errors added during failed parse
		if len(p.errors) > savedErrorCount {
			p.errors = p.errors[:savedErrorCount]
		}
		return nil
	}

	return typeArgs
}

// parseTypeArguments parses a comma-separated list of type arguments: <T, U, V>
func (p *Parser) parseTypeArguments() ([]Expression, error) {
	if !p.curTokenIs(lexer.LT) {
		return nil, fmt.Errorf("internal error: parseTypeArguments called without '<'")
	}

	var typeArgs []Expression

	// Move to first type argument
	p.nextToken()

	// Handle empty type argument list (not typically valid, but graceful handling)
	if p.curTokenIs(lexer.GT) {
		return typeArgs, nil // Empty list
	}

	// Parse first type argument
	firstArg := p.parseTypeExpression()
	if firstArg == nil {
		return nil, fmt.Errorf("failed to parse type argument")
	}
	typeArgs = append(typeArgs, firstArg)

	// Parse remaining type arguments
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken() // consume comma
		p.nextToken() // move to next type argument

		arg := p.parseTypeExpression()
		if arg == nil {
			return nil, fmt.Errorf("failed to parse type argument")
		}
		typeArgs = append(typeArgs, arg)
	}

	// Expect closing '>' - handle >> and >>> splitting
	if !p.expectPeekGT() {
		return nil, fmt.Errorf("expected '>' after type arguments")
	}

	return typeArgs, nil
}

// looksLikeGenericCall does a simple lookahead to determine if the current '<' token
// is likely the start of a generic call (identifier<Type>) rather than a comparison (a < b)
func (p *Parser) looksLikeGenericCall() bool {
	// We're currently at '<', peek should be the first token after it

	// String literals are valid type arguments (literal types like "x" | "y")
	if p.peekTokenIs(lexer.STRING) {
		return true
	}

	// Number literals can also be type arguments (literal types like 1 | 2 | 3)
	if p.peekTokenIs(lexer.NUMBER) {
		return true
	}

	// Parentheses can start a grouped type or tuple type
	if p.peekTokenIs(lexer.LPAREN) {
		return true
	}

	// Square bracket starts array/tuple type
	if p.peekTokenIs(lexer.LBRACKET) {
		return true
	}

	// Curly brace starts object type
	if p.peekTokenIs(lexer.LBRACE) {
		return true
	}

	// Keywords that can start type expressions
	if p.peekTokenIs(lexer.TYPEOF) || p.peekTokenIs(lexer.KEYOF) ||
		p.peekTokenIs(lexer.VOID) || p.peekTokenIs(lexer.NULL) ||
		p.peekTokenIs(lexer.UNDEFINED) || p.peekTokenIs(lexer.INFER) ||
		p.peekTokenIs(lexer.TRUE) || p.peekTokenIs(lexer.FALSE) {
		return true
	}

	// Identifiers that look like type names
	if p.peekTokenIs(lexer.IDENT) {
		typeName := p.peekToken.Literal

		// Common TypeScript type names (built-in types)
		if typeName == "string" || typeName == "number" || typeName == "boolean" ||
			typeName == "object" || typeName == "any" || typeName == "unknown" ||
			typeName == "void" || typeName == "never" || typeName == "undefined" ||
			typeName == "null" || typeName == "bigint" || typeName == "symbol" {
			return true
		}

		// Type names that start with a capital letter (e.g., Array, Map, Set, custom types)
		if len(typeName) > 0 && typeName[0] >= 'A' && typeName[0] <= 'Z' {
			return true
		}

		// Single-letter type parameters (common: T, U, V, K, V, etc.)
		if len(typeName) == 1 && typeName[0] >= 'A' && typeName[0] <= 'Z' {
			return true
		}

		// Reject lowercase multi-letter identifiers (likely variables in comparisons)
		return false
	}

	return false
}

// parseKeyofTypeExpression parses a keyof type expression like 'keyof T'
func (p *Parser) parseKeyofTypeExpression() Expression {
	kte := &KeyofTypeExpression{
		Token: p.curToken, // The 'keyof' token
	}

	// Move to the type expression after 'keyof'
	p.nextToken()

	// Parse the type that we're getting keys from
	kte.Type = p.parseTypeExpression()
	if kte.Type == nil {
		p.addError(p.curToken, "expected type expression after 'keyof'")
		return nil
	}

	return kte
}

// parseTypeofTypeExpression parses a typeof type expression like 'typeof someVariable'
func (p *Parser) parseTypeofTypeExpression() Expression {
	tte := &TypeofTypeExpression{
		Token: p.curToken, // The 'typeof' token
	}

	// Move to the identifier after 'typeof'
	p.nextToken()

	// The next token should be an identifier
	if p.curToken.Type != lexer.IDENT {
		p.addError(p.curToken, "expected identifier after 'typeof'")
		return nil
	}

	tte.Identifier = p.curToken.Literal

	return tte
}

// parseInferTypeExpression parses an infer type expression like 'infer R'
func (p *Parser) parseInferTypeExpression() Expression {
	ite := &InferTypeExpression{
		Token: p.curToken, // The 'infer' token
	}

	// Move to the type parameter name after 'infer'
	p.nextToken()

	// The next token should be an identifier (the type parameter name)
	if p.curToken.Type != lexer.IDENT {
		p.addError(p.curToken, "expected identifier after 'infer'")
		return nil
	}

	ite.TypeParameter = p.curToken.Literal

	return ite
}

// parseTemplateLiteralType parses template literal types like `Hello ${T}!`
func (p *Parser) parseTemplateLiteralType() Expression {
	tlte := &TemplateLiteralTypeExpression{Token: p.curToken} // TEMPLATE_START token
	tlte.Parts = []Node{}

	// Consume the opening backtick
	p.nextToken()

	// Always start with a string part (can be empty)
	expectingString := true

	for !p.curTokenIs(lexer.TEMPLATE_END) && !p.curTokenIs(lexer.EOF) {
		if p.curTokenIs(lexer.TEMPLATE_STRING) {
			if !expectingString {
				p.addError(p.curToken, "unexpected string in template literal type")
				return nil
			}
			// String part of the template (include both cooked and raw values)
			stringPart := &TemplateStringPart{
				Value:             p.curToken.Literal,
				Raw:               p.curToken.RawLiteral,
				CookedIsUndefined: p.curToken.CookedIsUndefined,
			}
			tlte.Parts = append(tlte.Parts, stringPart)
			expectingString = false
			p.nextToken()
		} else if p.curTokenIs(lexer.TEMPLATE_INTERPOLATION) {
			// If we were expecting a string but got interpolation, add empty string
			if expectingString {
				emptyString := &TemplateStringPart{Value: "", Raw: "", CookedIsUndefined: false}
				tlte.Parts = append(tlte.Parts, emptyString)
			}

			p.nextToken() // Move past ${

			// Parse the TYPE expression inside the interpolation (not value expression!)
			typeExpr := p.parseTypeExpression()
			if typeExpr == nil {
				p.addError(p.curToken, "failed to parse type expression in template literal type interpolation")
				return nil
			}
			tlte.Parts = append(tlte.Parts, typeExpr)

			// Expect closing brace }
			if !p.expectPeek(lexer.RBRACE) {
				p.addError(p.curToken, "expected '}' to close template literal type interpolation")
				return nil
			}
			p.nextToken()          // Move past }
			expectingString = true // After type expression, we expect a string
		} else {
			// Unexpected token
			p.addError(p.curToken, fmt.Sprintf("unexpected token in template literal type: %s", p.curToken.Type))
			return nil
		}
	}

	if !p.curTokenIs(lexer.TEMPLATE_END) {
		p.addError(p.curToken, "unterminated template literal type, expected closing backtick")
		return nil
	}

	// If we were expecting a string at the end, add empty string
	if expectingString {
		emptyString := &TemplateStringPart{Value: "", Raw: "", CookedIsUndefined: false}
		tlte.Parts = append(tlte.Parts, emptyString)
	}

	// Don't consume the closing backtick here - let the caller handle it
	return tlte
}

// parseTypePredicateExpression parses a type predicate like 'x is string'
func (p *Parser) parseTypePredicateExpression(left Expression) Expression {
	// left should be an identifier representing the parameter
	param, ok := left.(*Identifier)
	if !ok {
		p.addError(p.curToken, "type predicate parameter must be an identifier")
		return nil
	}

	tpe := &TypePredicateExpression{
		Token:     p.curToken, // The 'is' token
		Parameter: param,
	}

	// Move to the type expression after 'is'
	p.nextToken()

	// Parse the type that we're checking for
	tpe.Type = p.parseTypeExpression()
	if tpe.Type == nil {
		p.addError(p.curToken, "expected type expression after 'is' in type predicate")
		return nil
	}

	return tpe
}

// parseGenericCallOrComparison handles the ambiguity between generic calls (func<T>()) and comparisons (a < b)
func (p *Parser) parseGenericCallOrComparison(left Expression) Expression {
	// Try generic call parsing if left is an identifier or member expression
	// This handles both foo<T>() and obj.method<T>() patterns
	_, isIdent := left.(*Identifier)
	_, isMember := left.(*MemberExpression)

	if isIdent || isMember {
		// Check if this looks like a generic call by doing a simple lookahead
		// We need to look for pattern: callee < TypeExpr > (
		if p.looksLikeGenericCall() {
			// Save current state for potential backtracking (tokens and lexer)
			savedCur := p.curToken
			savedPeek := p.peekToken
			savedErrorCount := len(p.errors)
			lexerState := p.l.SaveState()

			// Try to parse type arguments (current token is '<')
			typeArgs, err := p.parseTypeArguments()
			if err == nil && typeArgs != nil && p.peekTokenIs(lexer.LPAREN) {
				// Success! This is a generic call: callee<types>(args)
				p.nextToken() // consume '('
				callExpr := &CallExpression{
					Token:         p.curToken, // The '(' token
					Function:      left,       // Can be Identifier or MemberExpression
					TypeArguments: typeArgs,
				}
				callExpr.Arguments = p.parseExpressionList(lexer.RPAREN)
				return callExpr
			} else {
				// Failed to parse as generic call, backtrack lexer and tokens, then parse as comparison
				p.l.RestoreState(lexerState)
				p.curToken = savedCur
				p.peekToken = savedPeek
				// Remove any errors added during failed parse
				if len(p.errors) > savedErrorCount {
					p.errors = p.errors[:savedErrorCount]
				}
			}
		}
	}

	// Fall back to regular infix expression (comparison)
	return p.parseInfixExpression(left)
}

// isMappedTypePattern looks ahead to determine if we're parsing a mapped type vs index signature
// Mapped type: [P in K] vs Index signature: [key: string]
// We use a simple temporary parser approach
func (p *Parser) isMappedTypePattern() bool {
	debugPrint("isMappedTypePattern: starting check from cur=%s, peek=%s", p.curToken.Literal, p.peekToken.Literal)

	// Simple logic: if we see '[' followed eventually by 'in', it's a mapped type
	// We'll create a temporary lexer instance to check without affecting our state

	// Save complete lexer state
	savedState := p.l.SaveState()

	// Skip optional modifiers and look for [ IDENT in pattern
	found := false

	// Start from peekToken position
	token := p.peekToken
	tokenCount := 0

	// Skip readonly/+/- modifiers
	for token.Type == lexer.READONLY || token.Type == lexer.PLUS || token.Type == lexer.MINUS {
		token = p.l.NextToken()
		tokenCount++
		if tokenCount > 10 { // Safety limit
			break
		}
	}

	// Expect '['
	if token.Type == lexer.LBRACKET {
		token = p.l.NextToken() // Get identifier
		tokenCount++

		// Expect identifier
		if token.Type == lexer.IDENT {
			token = p.l.NextToken() // Check for 'in'
			tokenCount++

			// Check for 'in' - this distinguishes mapped types from index signatures
			if token.Type == lexer.IN {
				found = true
			}
		}
	}

	// Restore complete lexer state
	p.l.RestoreState(savedState)

	debugPrint("isMappedTypePattern: result=%t", found)
	return found
}

// parseMappedTypeExpression parses mapped types like { [P in K]: T } or { readonly [P in keyof T]?: T[P] }
func (p *Parser) parseMappedTypeExpression(startToken lexer.Token) Expression {
	debugPrint("=== STARTING MAPPED TYPE PARSING ===")
	debugPrint("startToken: %s, cur: %s, peek: %s", startToken.Literal, p.curToken.Literal, p.peekToken.Literal)

	mappedType := &MappedTypeExpression{
		Token: startToken, // The '{' token
	}

	p.nextToken() // Move past '{'
	debugPrint("After moving past '{': cur: %s, peek: %s", p.curToken.Literal, p.peekToken.Literal)

	// Parse optional readonly modifier at the beginning
	if p.curTokenIs(lexer.PLUS) && p.peekTokenIs(lexer.READONLY) {
		mappedType.ReadonlyModifier = "+"
		p.nextToken() // Move to 'readonly'
		p.nextToken() // Move past 'readonly'
	} else if p.curTokenIs(lexer.MINUS) && p.peekTokenIs(lexer.READONLY) {
		mappedType.ReadonlyModifier = "-"
		p.nextToken() // Move to 'readonly'
		p.nextToken() // Move past 'readonly'
	} else if p.curTokenIs(lexer.READONLY) {
		mappedType.ReadonlyModifier = "+"
		p.nextToken() // Move past 'readonly'
	}

	// Expect '['
	if !p.curTokenIs(lexer.LBRACKET) {
		p.addError(p.curToken, "expected '[' in mapped type")
		return nil
	}

	// Expect type parameter (P in [P in K])
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	mappedType.TypeParameter = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	debugPrint("Parsed type parameter: %s, cur: %s, peek: %s", mappedType.TypeParameter.Value, p.curToken.Literal, p.peekToken.Literal)

	// Expect 'in'
	if !p.expectPeek(lexer.IN) {
		return nil
	}
	debugPrint("Found 'in', cur: %s, peek: %s", p.curToken.Literal, p.peekToken.Literal)

	// Parse constraint type (K in [P in K])
	p.nextToken() // Move to start of constraint type
	debugPrint("About to parse constraint type, cur: %s, peek: %s", p.curToken.Literal, p.peekToken.Literal)

	// Debug: check what token we're at
	if p.curToken.Type == lexer.EOF {
		p.addError(p.curToken, "unexpected EOF while parsing mapped type constraint")
		return nil
	}

	mappedType.ConstraintType = p.parseTypeExpression()
	if mappedType.ConstraintType == nil {
		return nil
	}
	debugPrint("Parsed constraint type, cur: %s, peek: %s", p.curToken.Literal, p.peekToken.Literal)

	// After parseTypeExpression, we should be positioned at the last token of the constraint
	// and peeking at ']'
	if !p.expectPeek(lexer.RBRACKET) {
		return nil
	}
	debugPrint("Found ']', cur: %s, peek: %s", p.curToken.Literal, p.peekToken.Literal)
	// Now: cur=']', peek=next token (could be '?', ':', etc.)

	// Parse optional '?' modifier
	if p.peekTokenIs(lexer.QUESTION) {
		mappedType.OptionalModifier = "+"
		p.nextToken() // Now: cur='?', peek=':'
		debugPrint("Found '?', cur: %s, peek: %s", p.curToken.Literal, p.peekToken.Literal)
	} else if p.peekTokenIs(lexer.MINUS) {
		// Check if it's -?
		if p.peekTokenIs2(lexer.QUESTION) {
			mappedType.OptionalModifier = "-"
			p.nextToken() // Move to '-'
			p.nextToken() // Move to '?'
			debugPrint("Found '-?', cur: %s, peek: %s", p.curToken.Literal, p.peekToken.Literal)
		}
	} else if p.peekTokenIs(lexer.PLUS) {
		// Check if it's +?
		if p.peekTokenIs2(lexer.QUESTION) {
			mappedType.OptionalModifier = "+"
			p.nextToken() // Move to '+'
			p.nextToken() // Move to '?'
			debugPrint("Found '+?', cur: %s, peek: %s", p.curToken.Literal, p.peekToken.Literal)
		}
	}

	// Now we should be peeking at ':' (or cur=? and peek=:)
	debugPrint("About to expect ':', cur: %s, peek: %s", p.curToken.Literal, p.peekToken.Literal)
	if !p.expectPeek(lexer.COLON) {
		return nil
	}
	debugPrint("Found ':', cur: %s, peek: %s", p.curToken.Literal, p.peekToken.Literal)
	// Now: cur=':', peek=value type start

	// Parse value type
	p.nextToken() // Move to start of value type
	debugPrint("About to parse value type, cur: %s, peek: %s", p.curToken.Literal, p.peekToken.Literal)
	mappedType.ValueType = p.parseTypeExpression()
	if mappedType.ValueType == nil {
		return nil
	}
	debugPrint("Parsed value type, cur: %s, peek: %s", p.curToken.Literal, p.peekToken.Literal)

	// Expect '}'
	if !p.expectPeek(lexer.RBRACE) {
		return nil
	}

	return mappedType
}

// ----------------------------------------------------------------------------
// Module System: Import/Export Parsing Methods
// ----------------------------------------------------------------------------
// parseImportDeclaration parses various import statement forms:
// import defaultImport from "module"
// import * as name from "module"
// import { export1, export2 } from "module"
// import { export1 as alias1 } from "module"
// import defaultImport, { export1, export2 } from "module"
// import defaultImport, * as name from "module"
func (p *Parser) parseImportDeclaration() *ImportDeclaration {
	stmt := &ImportDeclaration{Token: p.curToken}

	// Move to the next token to see what kind of import this is
	p.nextToken()

	// Check for type-only import: import type { ... } from "module"
	if p.curToken.Type == lexer.TYPE {
		stmt.IsTypeOnly = true
		p.nextToken() // consume 'type' keyword
	}

	// Check for bare import: import "module-name"
	if p.curToken.Type == lexer.STRING {
		// This is a bare import with no specifiers
		stmt.Source = &StringLiteral{
			Token: p.curToken,
			Value: p.curToken.Literal,
		}
		stmt.Specifiers = []ImportSpecifier{} // Empty specifiers for bare import

		// Parse import attributes for bare imports too
		if p.peekToken.Type == lexer.WITH {
			p.nextToken() // consume source string
			p.nextToken() // consume 'with'

			// parseImportAttributes expects current token to be before '{'
			stmt.Attributes = p.parseImportAttributes()
			if stmt.Attributes == nil {
				return nil
			}
		}

		// Optional semicolon
		if p.peekToken.Type == lexer.SEMICOLON {
			p.nextToken()
		}
		return stmt
	}

	// Parse import specifiers for non-bare imports
	var specifiers []ImportSpecifier

	// Check for different import patterns
	if p.curToken.Type == lexer.IDENT {
		// Default import: import defaultName from "module"
		// Or mixed: import defaultName, { named } from "module"
		// Or mixed: import defaultName, * as name from "module"
		defaultSpec := &ImportDefaultSpecifier{
			Token: p.curToken,
			Local: &Identifier{Token: p.curToken, Value: p.curToken.Literal},
		}
		specifiers = append(specifiers, defaultSpec)

		// Check for comma (mixed imports)
		if p.peekToken.Type == lexer.COMMA {
			p.nextToken() // consume identifier
			p.nextToken() // consume comma

			// Parse additional specifiers after comma
			additionalSpecs := p.parseImportSpecifierList()
			if additionalSpecs == nil {
				return nil
			}
			specifiers = append(specifiers, additionalSpecs...)
		}
	} else {
		// Parse non-default imports: { named }, * as namespace
		additionalSpecs := p.parseImportSpecifierList()
		if additionalSpecs == nil {
			return nil
		}
		specifiers = append(specifiers, additionalSpecs...)
	}

	stmt.Specifiers = specifiers

	// Expect 'from' keyword
	if !p.expectPeek(lexer.FROM) {
		return nil
	}

	// Parse source string
	if !p.expectPeek(lexer.STRING) {
		return nil
	}

	stmt.Source = &StringLiteral{
		Token: p.curToken,
		Value: p.curToken.Literal,
	}

	// Parse import attributes (import assertions): with { type: "json" }
	if p.peekToken.Type == lexer.WITH {
		p.nextToken() // consume source string
		p.nextToken() // consume 'with'

		// parseImportAttributes expects current token to be before '{'
		stmt.Attributes = p.parseImportAttributes()
		if stmt.Attributes == nil {
			return nil
		}
	}

	// Optional semicolon
	if p.peekToken.Type == lexer.SEMICOLON {
		p.nextToken()
	}

	return stmt
}

// parseImportAttributes parses import attributes: { type: "json", ... }
// Called after consuming 'with', current token should be '{'
func (p *Parser) parseImportAttributes() map[string]string {
	attributes := make(map[string]string)

	// Verify current token is '{'
	if p.curToken.Type != lexer.LBRACE {
		p.addError(p.curToken, fmt.Sprintf("Expected '{' to start import attributes, got %s", p.curToken.Type))
		return nil
	}

	// Move to first key or '}'
	p.nextToken()

	for p.curToken.Type != lexer.RBRACE {
		// Parse key (must be identifier, string, or certain keywords like 'type')
		var key string
		if p.curToken.Type == lexer.IDENT {
			key = p.curToken.Literal
		} else if p.curToken.Type == lexer.STRING {
			key = p.curToken.Literal
		} else if p.curToken.Type == lexer.TYPE {
			// Allow 'type' keyword as attribute key
			key = "type"
		} else {
			p.addError(p.curToken, fmt.Sprintf("Expected identifier or string for import attribute key, got %s", p.curToken.Type))
			return nil
		}

		// Expect colon
		if !p.expectPeek(lexer.COLON) {
			return nil
		}

		// Parse value (must be string literal)
		if !p.expectPeek(lexer.STRING) {
			p.addError(p.curToken, "Import attribute value must be a string literal")
			return nil
		}
		value := p.curToken.Literal

		attributes[key] = value

		// Check for comma or closing brace
		if p.peekToken.Type == lexer.COMMA {
			p.nextToken() // consume value
			p.nextToken() // consume comma
		} else if p.peekToken.Type == lexer.RBRACE {
			p.nextToken() // consume value
			break
		} else {
			p.addError(p.peekToken, fmt.Sprintf("Expected ',' or '}' in import attributes, got %s", p.peekToken.Type))
			return nil
		}
	}

	if p.curToken.Type != lexer.RBRACE {
		p.addError(p.curToken, "Expected '}' to close import attributes")
		return nil
	}

	return attributes
}

// parseImportSpecifierList parses { name1, name2 as alias } or * as namespace
func (p *Parser) parseImportSpecifierList() []ImportSpecifier {
	var specs []ImportSpecifier

	if p.curToken.Type == lexer.ASTERISK {
		// Namespace import: * as name
		if !p.expectPeek(lexer.AS) {
			return nil
		}
		if !p.expectPeek(lexer.IDENT) {
			return nil
		}

		namespaceSpec := &ImportNamespaceSpecifier{
			Token: p.curToken,
			Local: &Identifier{Token: p.curToken, Value: p.curToken.Literal},
		}
		specs = append(specs, namespaceSpec)

	} else if p.curToken.Type == lexer.LBRACE {
		// Named imports: { name1, name2 as alias, "string name" as alias, default as alias }
		p.nextToken() // consume '{'

		for {
			var imported *Identifier
			var importedToken lexer.Token

			// Check for type-only import: { type name }
			isTypeOnlySpecifier := false
			if p.curToken.Type == lexer.TYPE {
				isTypeOnlySpecifier = true
				p.nextToken() // consume 'type'
			}

			// Handle different import name patterns
			if p.curToken.Type == lexer.IDENT {
				// Regular identifier: { name } or { name as alias }
				imported = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
				importedToken = p.curToken
			} else if p.curToken.Type == lexer.STRING {
				// String named import: { "string name" as alias }
				imported = &Identifier{
					Token: p.curToken,
					Value: p.curToken.Literal, // Keep the quoted string
				}
				importedToken = p.curToken
			} else if p.curToken.Type == lexer.DEFAULT {
				// Default as alias: { default as alias }
				imported = &Identifier{Token: p.curToken, Value: "default"}
				importedToken = p.curToken
			} else {
				// Expected identifier, string, or default
				p.addError(p.curToken, "Expected identifier, string literal, or 'default' in import specifier")
				return nil
			}

			local := imported // Default: same as imported name

			// Check for 'as' alias
			if p.peekToken.Type == lexer.AS {
				p.nextToken() // consume imported name/string/default
				p.nextToken() // consume 'as'

				if p.curToken.Type != lexer.IDENT {
					p.peekError(lexer.IDENT)
					return nil
				}
				local = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
			} else if p.curToken.Type == lexer.STRING {
				// String names MUST have an alias
				p.addError(p.curToken, "String-named imports must use 'as' to provide an alias")
				return nil
			} else if p.curToken.Type == lexer.DEFAULT {
				// Default imports without alias need special handling
				// This should be allowed: { default } (imports as 'default')
				// But commonly you'd use: { default as something }
			}

			namedSpec := &ImportNamedSpecifier{
				Token:      importedToken,
				Imported:   imported,
				Local:      local,
				IsTypeOnly: isTypeOnlySpecifier,
			}
			specs = append(specs, namedSpec)

			// Check for more specifiers
			if p.peekToken.Type != lexer.COMMA {
				break
			}
			p.nextToken() // consume current identifier/alias
			p.nextToken() // consume comma

			// Next specifier should be identifier, string, default, or type
			if p.curToken.Type != lexer.IDENT && p.curToken.Type != lexer.STRING &&
				p.curToken.Type != lexer.DEFAULT && p.curToken.Type != lexer.TYPE {
				p.addError(p.curToken, "Expected identifier, string literal, 'default', or 'type' in import specifier")
				return nil
			}
		}

		// Expect closing brace
		if !p.expectPeek(lexer.RBRACE) {
			return nil
		}
	}

	return specs
}

// parseExportDeclaration parses various export statement forms:
// export const x = 1;
// export function foo() {}
// export { name1, name2 };
// export { name1 as alias };
// export { name1 } from "module";
// export default expression;
// export * from "module";
// export * as name from "module";
func (p *Parser) parseExportDeclaration() Statement {
	exportToken := p.curToken

	// Move to the next token to see what kind of export this is
	p.nextToken()

	// Check for type-only export: export type { ... } from "module" or export type * from "module"
	// But distinguish from type alias: export type TypeAlias = ...
	isTypeOnly := false
	if p.curToken.Type == lexer.TYPE {
		// Look ahead to see if this is a re-export or type alias
		if p.peekTokenIs(lexer.LBRACE) || p.peekTokenIs(lexer.ASTERISK) {
			// This is a re-export: export type { ... } or export type * from "module"
			isTypeOnly = true
			p.nextToken() // consume 'type' keyword
		}
		// If peek is not '{' or '*', this is a type alias declaration, so don't consume 'type'
		// Let it fall through to the case statement which will handle lexer.TYPE
	}

	switch p.curToken.Type {
	case lexer.DEFAULT:
		// export default expression;
		return p.parseExportDefaultDeclaration(exportToken)

	case lexer.ASTERISK:
		// export * from "module" or export * as name from "module" or export type * from "module"
		return p.parseExportAllDeclaration(exportToken, isTypeOnly)

	case lexer.LBRACE:
		// export { name1, name2 } or export { name1 } from "module"
		return p.parseExportNamedDeclarationWithSpecifiers(exportToken, isTypeOnly)

	case lexer.CONST, lexer.LET, lexer.VAR, lexer.FUNCTION, lexer.CLASS, lexer.INTERFACE, lexer.TYPE, lexer.ENUM:
		// export const x = 1; export function foo() {}
		return p.parseExportNamedDeclarationWithDeclaration(exportToken)

	default:
		// Should not reach here due to expectPeek checks above
		return nil
	}
}

// parseExportDefaultDeclaration parses: export default expression;
func (p *Parser) parseExportDefaultDeclaration(exportToken lexer.Token) *ExportDefaultDeclaration {
	stmt := &ExportDefaultDeclaration{Token: exportToken}

	// Parse the default export expression
	p.nextToken() // Move past 'default'
	stmt.Declaration = p.parseExpression(LOWEST)
	if stmt.Declaration == nil {
		return nil
	}

	// Optional semicolon
	if p.peekToken.Type == lexer.SEMICOLON {
		p.nextToken()
	}

	return stmt
}

// parseExportAllDeclaration parses: export * from "module" or export * as name from "module" or export type * from "module"
func (p *Parser) parseExportAllDeclaration(exportToken lexer.Token, isTypeOnly bool) *ExportAllDeclaration {
	stmt := &ExportAllDeclaration{Token: exportToken, IsTypeOnly: isTypeOnly}

	// Check for optional 'as name'
	if p.peekToken.Type == lexer.AS {
		p.nextToken() // consume '*'
		p.nextToken() // consume 'as'

		if p.curToken.Type != lexer.IDENT {
			p.peekError(lexer.IDENT)
			return nil
		}

		stmt.Exported = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

	// Expect 'from' keyword
	if !p.expectPeek(lexer.FROM) {
		return nil
	}

	// Parse source string
	if !p.expectPeek(lexer.STRING) {
		return nil
	}

	stmt.Source = &StringLiteral{
		Token: p.curToken,
		Value: p.curToken.Literal,
	}

	// Optional semicolon
	if p.peekToken.Type == lexer.SEMICOLON {
		p.nextToken()
	}

	return stmt
}

// isExportSpecifierName checks if a token can be used as an export specifier name
// In ES modules, export specifier names can be identifiers, strings, or certain keywords
func isExportSpecifierName(t lexer.TokenType) bool {
	switch t {
	case lexer.IDENT, lexer.STRING:
		return true
	// Keywords that can be used as export names
	case lexer.DEFAULT, lexer.AS, lexer.FROM,
		lexer.IF, lexer.ELSE, lexer.FOR, lexer.WHILE, lexer.DO,
		lexer.SWITCH, lexer.CASE, lexer.BREAK, lexer.CONTINUE, lexer.RETURN,
		lexer.THROW, lexer.TRY, lexer.CATCH, lexer.FINALLY,
		lexer.FUNCTION, lexer.CLASS, lexer.CONST, lexer.LET, lexer.VAR,
		lexer.NEW, lexer.DELETE, lexer.TYPEOF, lexer.VOID, lexer.IN, lexer.INSTANCEOF,
		lexer.THIS, lexer.SUPER, lexer.NULL, lexer.TRUE, lexer.FALSE,
		lexer.IMPORT, lexer.EXPORT, lexer.EXTENDS, lexer.IMPLEMENTS,
		lexer.STATIC, lexer.GET, lexer.SET, lexer.ASYNC, lexer.AWAIT, lexer.YIELD,
		lexer.TYPE, lexer.INTERFACE, lexer.ENUM,
		lexer.PRIVATE, lexer.PROTECTED, lexer.PUBLIC, lexer.READONLY, lexer.ABSTRACT,
		lexer.KEYOF, lexer.INFER, lexer.IS:
		return true
	}
	return false
}

// parseExportSpecifierExpr parses an export specifier name as an expression
func (p *Parser) parseExportSpecifierExpr() Expression {
	if p.curToken.Type == lexer.STRING {
		return &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
	}
	// For all other valid tokens (identifiers and keywords), create an identifier
	return &Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

// parseExportNamedDeclarationWithSpecifiers parses: export { name1, name2 } [from "module"]
func (p *Parser) parseExportNamedDeclarationWithSpecifiers(exportToken lexer.Token, isTypeOnly bool) *ExportNamedDeclaration {
	stmt := &ExportNamedDeclaration{Token: exportToken, IsTypeOnly: isTypeOnly}

	// Parse export specifiers - first specifier can be identifier, string, or keyword
	p.nextToken() // move past '{'
	if !isExportSpecifierName(p.curToken.Type) {
		p.addError(p.curToken, "expected identifier or string in export specifier")
		return nil
	}

	var specifiers []ExportSpecifier

	for {
		// Parse export specifier - can be identifier, string literal, or keyword
		var local Expression
		var exported Expression

		if !isExportSpecifierName(p.curToken.Type) {
			p.addError(p.curToken, "expected identifier or string in export specifier")
			return nil
		}
		local = p.parseExportSpecifierExpr()
		exported = local // Default: same as local name

		// Check for 'as' alias
		if p.peekToken.Type == lexer.AS {
			p.nextToken() // consume local name
			p.nextToken() // consume 'as'

			// Exported name can be identifier, string literal, or keyword
			if !isExportSpecifierName(p.curToken.Type) {
				p.addError(p.curToken, "expected identifier or string after 'as' in export specifier")
				return nil
			}
			exported = p.parseExportSpecifierExpr()
		}

		namedSpec := &ExportNamedSpecifier{
			Token:    p.curToken,
			Local:    local,
			Exported: exported,
		}
		specifiers = append(specifiers, namedSpec)

		// Check for more specifiers
		if p.peekToken.Type != lexer.COMMA {
			break
		}
		p.nextToken() // consume current identifier/alias
		p.nextToken() // consume comma

		if !isExportSpecifierName(p.curToken.Type) {
			p.addError(p.curToken, "expected identifier or string in export specifier")
			return nil
		}
	}

	stmt.Specifiers = specifiers

	// Expect closing brace
	if !p.expectPeek(lexer.RBRACE) {
		return nil
	}

	// Check for optional 'from' clause
	if p.peekToken.Type == lexer.FROM {
		p.nextToken() // consume '}'
		p.nextToken() // consume 'from'

		if p.curToken.Type != lexer.STRING {
			p.peekError(lexer.STRING)
			return nil
		}

		stmt.Source = &StringLiteral{
			Token: p.curToken,
			Value: p.curToken.Literal,
		}
	}

	// Optional semicolon
	if p.peekToken.Type == lexer.SEMICOLON {
		p.nextToken()
	}

	return stmt
}

// parseExportNamedDeclarationWithDeclaration parses: export const x = 1; export function foo() {}
func (p *Parser) parseExportNamedDeclarationWithDeclaration(exportToken lexer.Token) *ExportNamedDeclaration {
	stmt := &ExportNamedDeclaration{Token: exportToken}

	// Parse the declaration statement
	declaration := p.parseStatement()
	if declaration == nil {
		return nil
	}

	stmt.Declaration = declaration
	return stmt
}

// parseLeadingPipeUnionType handles union types that start with | like:
// | A | B | C
// This is equivalent to A | B | C but with leading pipe formatting
func (p *Parser) parseLeadingPipeUnionType() Expression {
	// Current token is the first '|'
	// Move to the first type after the leading pipe
	p.nextToken()

	// Parse the first type
	leftType := p.parseTypeExpression()
	if leftType == nil {
		p.addError(p.curToken, "expected type after leading '|'")
		return nil
	}

	// Now check if there are more union members (more | tokens)
	for p.peekTokenIs(lexer.PIPE) {
		p.nextToken() // consume the '|'

		unionExp := &UnionTypeExpression{
			Token: p.curToken, // The '|' token
			Left:  leftType,
		}

		p.nextToken() // move to the right-hand type
		unionExp.Right = p.parseTypeExpression()
		if unionExp.Right == nil {
			p.addError(p.curToken, "expected type after '|'")
			return nil
		}

		leftType = unionExp // Set up for potential next iteration
	}

	return leftType
}

// parseEnumMemberTypeExpression handles enum member type access like 'Color.Red' in type context
func (p *Parser) parseEnumMemberTypeExpression(left Expression) Expression {
	debugPrint("parseEnumMemberTypeExpression: left=%T, cur='%s'", left, p.curToken.Literal)

	// Current token should be DOT
	if !p.curTokenIs(lexer.DOT) {
		msg := fmt.Sprintf("internal error: parseEnumMemberTypeExpression called on non-DOT token %s", p.curToken.Type)
		p.addError(p.curToken, msg)
		return nil
	}

	// Left side should be an identifier (the enum name)
	enumName, ok := left.(*Identifier)
	if !ok {
		p.addError(p.curToken, "enum member type access requires identifier before '.'")
		return nil
	}

	dotToken := p.curToken

	// Move to the member name
	p.nextToken()

	// Member name should be an identifier
	if !p.curTokenIs(lexer.IDENT) {
		p.addError(p.curToken, fmt.Sprintf("expected member name after '.', got %s", p.curToken.Type))
		return nil
	}

	memberName := &Identifier{
		Token: p.curToken,
		Value: p.curToken.Literal,
	}

	// Create a member expression to represent EnumName.MemberName in type context
	memberExpr := &MemberExpression{
		Token:    dotToken,
		Object:   enumName,
		Property: memberName,
	}

	debugPrint("parseEnumMemberTypeExpression: created %s.%s", enumName.Value, memberName.Value)
	return memberExpr
}

// parseObjectTypeBracketProperty parses both index signatures and computed property names in object types
func (p *Parser) parseObjectTypeBracketProperty() *ObjectTypeProperty {
	// We're currently at '[', move to the content
	p.nextToken()

	// Parse the expression inside the brackets
	expr := p.parseExpression(LOWEST)
	if expr == nil {
		return nil
	}

	// Look ahead to see if this is an index signature [key: type]: valueType
	// or a computed property [expr]: type
	if p.peekTokenIs(lexer.COLON) {
		// This is an index signature: [key: type]: valueType
		// The expression should be an identifier
		ident, ok := expr.(*Identifier)
		if !ok {
			p.addError(p.curToken, "index signature key must be an identifier")
			return nil
		}

		// Continue parsing as index signature
		prop := &ObjectTypeProperty{
			IsIndexSignature: true,
			KeyName:          ident,
		}

		// Expect ':'
		if !p.expectPeek(lexer.COLON) {
			return nil
		}

		// Parse key type
		p.nextToken() // Move to the start of the key type expression
		prop.KeyType = p.parseTypeExpression()
		if prop.KeyType == nil {
			return nil
		}

		// Expect ']'
		if !p.expectPeek(lexer.RBRACKET) {
			return nil
		}

		// Expect ':'
		if !p.expectPeek(lexer.COLON) {
			return nil
		}

		// Parse value type
		p.nextToken() // Move to the start of the value type expression
		prop.ValueType = p.parseTypeExpression()
		if prop.ValueType == nil {
			return nil
		}

		return prop
	} else if p.peekTokenIs(lexer.RBRACKET) {
		// This is a computed property: [expr]: type
		prop := &ObjectTypeProperty{
			IsComputedProperty: true,
			ComputedName:       expr,
		}

		// Expect ']'
		if !p.expectPeek(lexer.RBRACKET) {
			return nil
		}

		// Check for optional marker '?'
		if p.peekTokenIs(lexer.QUESTION) {
			p.nextToken() // Consume '?'
			prop.Optional = true
		}

		// Expect ':'
		if !p.expectPeek(lexer.COLON) {
			return nil
		}

		// Parse property type
		p.nextToken() // Move to the start of the type expression
		prop.Type = p.parseTypeExpression()
		if prop.Type == nil {
			return nil
		}

		return prop
	} else {
		p.addError(p.curToken, "expected ':' or ']' after bracket expression in object type")
		return nil
	}
}

// isGetterMethod checks if the current 'get' token is part of a getter method
// Returns true for: get foo() or get [computed]()
// Returns false for: get: value
func (p *Parser) isGetterMethod() bool {
	// Look ahead to see what follows 'get'
	if p.peekTokenIs(lexer.COLON) {
		// get: value - this is a regular property, not a getter
		return false
	}
	// get identifier(...) or get [computed](...) or get keywordAsIdentifier(...) - this is a getter method
	return p.peekTokenIs(lexer.IDENT) || p.peekTokenIs(lexer.STRING) || p.peekTokenIs(lexer.NUMBER) ||
		p.peekTokenIs(lexer.BIGINT) || p.peekTokenIs(lexer.LBRACKET) ||
		p.isKeywordThatCanBeIdentifier(p.peekToken.Type)
}

// isSetterMethod checks if the current 'set' token is part of a setter method
// Returns true for: set foo(param) or set [computed](param)
// Returns false for: set: value
func (p *Parser) isSetterMethod() bool {
	// Look ahead to see what follows 'set'
	if p.peekTokenIs(lexer.COLON) {
		// set: value - this is a regular property, not a setter
		return false
	}
	// set identifier(...) or set [computed](...) or set keywordAsIdentifier(...) - this is a setter method
	return p.peekTokenIs(lexer.IDENT) || p.peekTokenIs(lexer.STRING) || p.peekTokenIs(lexer.NUMBER) ||
		p.peekTokenIs(lexer.BIGINT) || p.peekTokenIs(lexer.LBRACKET) ||
		p.isKeywordThatCanBeIdentifier(p.peekToken.Type)
}
