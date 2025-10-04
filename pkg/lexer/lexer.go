package lexer

import (
	"fmt"
	"paserati/pkg/source"
	"strconv" // Added for ParseInt
	"strings" // Added for strings.Builder
	"unicode"
	"unicode/utf8"
)

// Debug flags
const debugLexer = false

// templateState represents the state of a template literal context
type templateState struct {
	inTemplate    bool
	braceDepth    int
	templateStart int
}

// LexerState captures the complete lexer state for backtracking
type LexerState struct {
	Position      int
	ReadPosition  int
	Ch            byte
	Line          int
	Column        int
	InTemplate    bool
	BraceDepth    int
	TemplateStart int
	TemplateStack []templateState
	PushedToken   *Token
}

// --- Debug Flag ---
const lexerDebug = false

func debugPrintf(format string, args ...interface{}) {
	if lexerDebug {
		fmt.Printf("[Lexer Debug] "+format+"\n", args...)
	}
}

// --- End Debug Flag ---

// TokenType represents the type of a token.
type TokenType string

// Token represents a lexical token.
type Token struct {
	Type     TokenType
	Literal  string // The actual text of the token (lexeme)
	Line     int    // 1-based line number where the token starts
	Column   int    // 1-based column number (rune index) where the token starts
	StartPos int    // 0-based byte offset where the token starts
	EndPos   int    // 0-based byte offset after the token ends
}

// --- Token Types ---
const (
	// Special
	ILLEGAL TokenType = "ILLEGAL" // Unknown token/character
	EOF     TokenType = "EOF"     // End Of File

	// Identifiers + Literals
	IDENT         TokenType = "IDENT"         // functionName, variableName
	PRIVATE_IDENT TokenType = "PRIVATE_IDENT" // #privateName
	NUMBER        TokenType = "NUMBER"        // 123, 45.67
	BIGINT        TokenType = "BIGINT"        // 123n
	STRING        TokenType = "STRING"        // "hello world"
	REGEX_LITERAL TokenType = "REGEX_LITERAL" // /pattern/flags
	NULL          TokenType = "NULL"          // Added
	UNDEFINED     TokenType = "UNDEFINED"     // Added

	// --- NEW: Template Literal Tokens ---
	TEMPLATE_START         TokenType = "TEMPLATE_START"         // ` (opening backtick)
	TEMPLATE_STRING        TokenType = "TEMPLATE_STRING"        // string parts between interpolations
	TEMPLATE_INTERPOLATION TokenType = "TEMPLATE_INTERPOLATION" // ${ (start of interpolation)
	TEMPLATE_END           TokenType = "TEMPLATE_END"           // ` (closing backtick)

	// Operators (add more later)
	ASSIGN   TokenType = "="
	PLUS     TokenType = "+"
	MINUS    TokenType = "-"
	BANG     TokenType = "!"
	ASTERISK TokenType = "*"
	SLASH    TokenType = "/"
	LT       TokenType = "<"
	GT       TokenType = ">"
	EQ       TokenType = "=="
	NOT_EQ   TokenType = "!="
	LE       TokenType = "<="
	GE       TokenType = ">="  // Added (assuming GT might become GE)
	DOT      TokenType = "."   // Added for member access
	SPREAD   TokenType = "..." // Added for spread/rest

	// Compound Assignment
	PLUS_ASSIGN     TokenType = "+=" // Added
	MINUS_ASSIGN    TokenType = "-=" // Added
	ASTERISK_ASSIGN TokenType = "*=" // Added
	SLASH_ASSIGN    TokenType = "/=" // Added

	// --- NEW: Remainder/Exponent Operators ---
	REMAINDER TokenType = "%"
	EXPONENT  TokenType = "**"

	// --- NEW: Remainder/Exponent Assign ---
	REMAINDER_ASSIGN TokenType = "%="
	EXPONENT_ASSIGN  TokenType = "**="

	// Increment/Decrement
	INC TokenType = "++" // Added
	DEC TokenType = "--" // Added

	// --- NEW: Bitwise Operators ---
	BITWISE_AND TokenType = "&"
	// BITWISE_OR           TokenType = "|" // Note: This might conflict with PIPE for Union Types if not handled carefully
	BITWISE_XOR          TokenType = "^"
	BITWISE_NOT          TokenType = "~"
	LEFT_SHIFT           TokenType = "<<"
	RIGHT_SHIFT          TokenType = ">>"
	UNSIGNED_RIGHT_SHIFT TokenType = ">>>"

	// --- NEW: Bitwise Assignment ---
	BITWISE_AND_ASSIGN          TokenType = "&="
	BITWISE_OR_ASSIGN           TokenType = "|="
	BITWISE_XOR_ASSIGN          TokenType = "^="
	LEFT_SHIFT_ASSIGN           TokenType = "<<="
	RIGHT_SHIFT_ASSIGN          TokenType = ">>="
	UNSIGNED_RIGHT_SHIFT_ASSIGN TokenType = ">>>="

	// --- NEW: Logical Assignment ---
	LOGICAL_AND_ASSIGN TokenType = "&&="
	LOGICAL_OR_ASSIGN  TokenType = "||="
	COALESCE_ASSIGN    TokenType = "??="

	// Type Operator
	PIPE TokenType = "|" // Added for Union Types - Retain for clarity, but NextToken needs careful handling

	// Delimiters
	COMMA     TokenType = ","
	SEMICOLON TokenType = ";"
	COLON     TokenType = ":"
	LPAREN    TokenType = "("
	RPAREN    TokenType = ")"
	LBRACE    TokenType = "{"
	RBRACE    TokenType = "}"
	LBRACKET  TokenType = "["  // Added for Arrays
	RBRACKET  TokenType = "]"  // Added for Arrays
	ARROW     TokenType = "=>" // Added for arrow functions

	// Keywords
	FUNCTION TokenType = "FUNCTION"
	LET      TokenType = "LET"
	CONST    TokenType = "CONST"
	VAR      TokenType = "VAR" // Added
	TRUE     TokenType = "TRUE"
	FALSE    TokenType = "FALSE"
	IF       TokenType = "IF"
	ELSE     TokenType = "ELSE"
	RETURN   TokenType = "RETURN"
	WHILE    TokenType = "WHILE"
	DO       TokenType = "DO" // Added for do...while
	FOR      TokenType = "FOR"
	WITH     TokenType = "WITH"     // Added for with statements
	BREAK    TokenType = "BREAK"    // Added
	CONTINUE TokenType = "CONTINUE" // Added
	TYPE     TokenType = "TYPE"     // Added for type aliases
	SWITCH   TokenType = "SWITCH"   // Added for switch statements
	CASE     TokenType = "CASE"     // Added for switch statements
	DEFAULT  TokenType = "DEFAULT"  // Added for switch statements
	TYPEOF   TokenType = "TYPEOF"   // Added for typeof operator
	VOID     TokenType = "VOID"     // Added for void operator
	KEYOF    TokenType = "KEYOF"    // Added for keyof operator
	INFER    TokenType = "INFER"    // Added for infer keyword in conditional types
	IS       TokenType = "IS"       // Added for type predicates (x is Type)

	// Logical Operators
	LOGICAL_AND TokenType = "&&" // Added
	LOGICAL_OR  TokenType = "||" // Added
	COALESCE    TokenType = "??" // Added

	// New Strict Equality Operators
	STRICT_EQ     TokenType = "==="
	STRICT_NOT_EQ TokenType = "!=="

	// New Ternary Operator Tokens
	QUESTION TokenType = "?"

	// Optional Chaining
	OPTIONAL_CHAINING TokenType = "?."

	// This keyword
	THIS TokenType = "THIS"
	// NEW keyword
	NEW TokenType = "NEW"
	// INTERFACE keyword
	INTERFACE TokenType = "INTERFACE"
	// EXTENDS keyword
	EXTENDS TokenType = "EXTENDS"
	// IMPLEMENTS keyword
	IMPLEMENTS TokenType = "IMPLEMENTS"
	// SUPER keyword
	SUPER TokenType = "SUPER"
	// OF keyword
	OF TokenType = "OF"
	// AS keyword
	AS TokenType = "AS"
	// SATISFIES keyword
	SATISFIES TokenType = "SATISFIES"
	// IN keyword
	IN TokenType = "IN"
	// INSTANCEOF keyword
	INSTANCEOF TokenType = "INSTANCEOF"
	// DELETE keyword
	DELETE TokenType = "DELETE"
	// Exception handling keywords
	TRY     TokenType = "TRY"
	CATCH   TokenType = "CATCH"
	THROW   TokenType = "THROW"
	FINALLY TokenType = "FINALLY"
	// Class keyword
	CLASS TokenType = "CLASS"
	// Enum keyword
	ENUM TokenType = "ENUM"
	// Static keyword (for future use)
	STATIC TokenType = "STATIC"
	// Readonly keyword
	READONLY TokenType = "READONLY"
	// Access modifier keywords
	PUBLIC    TokenType = "PUBLIC"
	PRIVATE   TokenType = "PRIVATE"
	PROTECTED TokenType = "PROTECTED"
	// Abstract and override keywords
	ABSTRACT TokenType = "ABSTRACT"
	OVERRIDE TokenType = "OVERRIDE"
	// Getter/Setter keywords
	GET TokenType = "GET"
	SET TokenType = "SET"
	// Module keywords
	IMPORT TokenType = "IMPORT"
	EXPORT TokenType = "EXPORT"
	FROM   TokenType = "FROM"
	// Generator keyword
	YIELD TokenType = "YIELD"
)

var keywords = map[string]TokenType{
	"function":   FUNCTION,
	"let":        LET,
	"var":        VAR, // Added
	"const":      CONST,
	"true":       TRUE,
	"false":      FALSE,
	"if":         IF,
	"else":       ELSE,
	"return":     RETURN,
	"null":       NULL,
	"undefined":  UNDEFINED, // Added
	"while":      WHILE,
	"do":         DO, // Added for do...while
	"for":        FOR,
	"with":       WITH,       // Added for with statements
	"break":      BREAK,      // Added
	"continue":   CONTINUE,   // Added
	"type":       TYPE,       // Added
	"switch":     SWITCH,     // Added
	"case":       CASE,       // Added
	"default":    DEFAULT,    // Added
	"this":       THIS,       // Added for this keyword
	"new":        NEW,        // Added for NEW keyword
	"interface":  INTERFACE,  // Added for interface keyword
	"extends":    EXTENDS,    // Added for extends keyword
	"implements": IMPLEMENTS, // Added for implements keyword
	"super":      SUPER,      // Added for super keyword
	"typeof":     TYPEOF,     // Added for typeof operator
	"void":       VOID,       // Added for void operator
	"keyof":      KEYOF,      // Added for keyof operator
	"infer":      INFER,      // Added for infer keyword
	"is":         IS,         // Added for type predicates
	"of":         OF,         // Added for for...of loops
	"as":         AS,         // Added for type assertions
	"satisfies":  SATISFIES,  // Added for satisfies operator
	"in":         IN,         // Added for in operator
	"instanceof": INSTANCEOF, // Added for instanceof operator
	"delete":     DELETE,     // Added for delete operator
	"try":        TRY,        // Added for try statements
	"catch":      CATCH,      // Added for catch blocks
	"throw":      THROW,      // Added for throw statements
	"finally":    FINALLY,    // Added for finally blocks
	"class":      CLASS,      // Added for class declarations/expressions
	"enum":       ENUM,       // Added for enum declarations
	"static":     STATIC,     // Added for static members
	"readonly":   READONLY,   // Added for readonly modifier
	"public":     PUBLIC,     // Added for public access modifier
	"private":    PRIVATE,    // Added for private access modifier
	"protected":  PROTECTED,  // Added for protected access modifier
	"abstract":   ABSTRACT,   // Added for abstract classes and methods
	"override":   OVERRIDE,   // Added for method overriding
	"get":        GET,        // Added for getter methods
	"set":        SET,        // Added for setter methods
	"import":     IMPORT,     // Added for import statements
	"export":     EXPORT,     // Added for export statements
	"from":       FROM,       // Added for import from clauses
	"yield":      YIELD,      // Added for generator yield expressions
}

// LookupIdent checks the keywords table for an identifier.
func LookupIdent(ident string) TokenType {
	if tokType, ok := keywords[ident]; ok {
		return tokType
	}
	return IDENT
}

// Lexer holds the state of the scanner.
type Lexer struct {
	source       *source.SourceFile // source file being lexed
	input        string             // source content (same as source.Content)
	position     int                // current position in input (points to current char's byte offset)
	readPosition int                // current reading position in input (byte offset after current char)
	ch           byte               // current char under examination
	line         int                // current 1-based line number
	column       int                // current 1-based column number (position of l.position on l.line)

	// --- NEW: Template literal state tracking ---
	inTemplate    bool // true when we're inside a template literal
	braceDepth    int  // tracks nested braces inside ${...} interpolations
	templateStart int  // position where current template started (for error reporting)

	// --- NEW: Template stack for nested template literals ---
	templateStack []templateState // stack to handle nested template literals

	// --- NEW: Token pushback for >> splitting in generics ---
	pushedToken *Token // Single token pushback buffer

	// --- NEW: Previous token tracking for regex context determination ---
	prevToken TokenType // tracks the previous token type to determine if '/' starts a regex
}

// CurrentPosition returns the lexer's current byte position in the input.
// Needed for parser backtracking.
func (l *Lexer) CurrentPosition() int {
	return l.position
}

// SetPosition resets the lexer to a specific byte position and re-reads the character.
// Needed for parser backtracking.
// Warning: Does not recalculate line numbers accurately if jumping significantly.
// Assumes backtracking is local and line changes are minimal or irrelevant for the backtrack.
// SaveState captures the current lexer state for backtracking
func (l *Lexer) SaveState() LexerState {
	// Make a deep copy of the template stack
	stackCopy := make([]templateState, len(l.templateStack))
	copy(stackCopy, l.templateStack)

	return LexerState{
		Position:      l.position,
		ReadPosition:  l.readPosition,
		Ch:            l.ch,
		Line:          l.line,
		Column:        l.column,
		InTemplate:    l.inTemplate,
		BraceDepth:    l.braceDepth,
		TemplateStart: l.templateStart,
		TemplateStack: stackCopy,
		PushedToken:   l.pushedToken, // Note: shallow copy of token pointer
	}
}

// RestoreState restores the lexer to a previously saved state
func (l *Lexer) RestoreState(state LexerState) {
	l.position = state.Position
	l.readPosition = state.ReadPosition
	l.ch = state.Ch
	l.line = state.Line
	l.column = state.Column
	l.inTemplate = state.InTemplate
	l.braceDepth = state.BraceDepth
	l.templateStart = state.TemplateStart
	l.templateStack = state.TemplateStack
	l.pushedToken = state.PushedToken
}

// SetPosition sets lexer position (legacy method, use SaveState/RestoreState for proper backtracking)
func (l *Lexer) SetPosition(pos int) {
	if pos < 0 {
		pos = 0
	}
	if pos >= len(l.input) {
		l.position = len(l.input)
		l.readPosition = len(l.input)
		l.ch = 0 // EOF
		return
	}
	l.position = pos
	l.readPosition = pos + 1
	l.ch = l.input[l.position]
	// NOTE: Line number is NOT recalculated here. Backtracking assumes it's okay.

	// DEPRECATED: This method resets template state which breaks backtracking.
	// Use SaveState/RestoreState instead for proper template literal handling.
	l.inTemplate = false
	l.braceDepth = 0
	l.templateStart = 0
	l.templateStack = nil
}

// NewLexer creates a new Lexer.
func NewLexer(input string) *Lexer {
	// Preprocess Unicode escapes, but preserve them in strings and comments
	input = PreprocessUnicodeEscapesContextAware(input)

	// Create a default source file for backward compatibility
	sourceFile := source.NewEvalSource(input)
	return NewLexerWithSource(sourceFile)
}

func NewLexerWithSource(sourceFile *source.SourceFile) *Lexer {
	l := &Lexer{
		source:    sourceFile,
		input:     sourceFile.Content,
		line:      1,
		column:    1,
		prevToken: ILLEGAL, // Initialize to ILLEGAL to allow regex at start of input
	} // Start at line 1, column 1
	l.readChar() // Initialize l.ch, l.position, l.readPosition, and potentially update line/column if input starts with newline
	return l
}

// newToken creates a new token with the current lexer state
func (l *Lexer) newToken(tokenType TokenType, literal string) Token {
	return Token{
		Type:     tokenType,
		Literal:  literal,
		Line:     l.line,
		Column:   l.column,
		StartPos: l.position,
		EndPos:   l.position + len(literal),
	}
}

// SplitRightShiftToken converts a >> token into > and pushes the second > back
// This is used for nested generics like Array<Array<T>>
func (l *Lexer) SplitRightShiftToken(rsToken Token) Token {
	// Create the first > token
	firstGT := Token{
		Type:     GT,
		Literal:  ">",
		Line:     rsToken.Line,
		Column:   rsToken.Column,
		StartPos: rsToken.StartPos,
		EndPos:   rsToken.StartPos + 1,
	}

	// Create the second > token and push it back
	secondGT := Token{
		Type:     GT,
		Literal:  ">",
		Line:     rsToken.Line,
		Column:   rsToken.Column + 1,
		StartPos: rsToken.StartPos + 1,
		EndPos:   rsToken.EndPos,
	}

	l.pushedToken = &secondGT
	debugPrintf("SplitRightShiftToken: split >> into > and pushed > back")

	return firstGT
}

// SplitUnsignedRightShiftToken converts a >>> token into > and pushes >> back
func (l *Lexer) SplitUnsignedRightShiftToken(ursToken Token) Token {
	// Create the first > token
	firstGT := Token{
		Type:     GT,
		Literal:  ">",
		Line:     ursToken.Line,
		Column:   ursToken.Column,
		StartPos: ursToken.StartPos,
		EndPos:   ursToken.StartPos + 1,
	}

	// Create the remaining >> token and push it back
	remainingRS := Token{
		Type:     RIGHT_SHIFT,
		Literal:  ">>",
		Line:     ursToken.Line,
		Column:   ursToken.Column + 1,
		StartPos: ursToken.StartPos + 1,
		EndPos:   ursToken.EndPos,
	}

	l.pushedToken = &remainingRS
	debugPrintf("SplitUnsignedRightShiftToken: split >>> into > and pushed >> back")

	return firstGT
}

// readChar gives us the next character and advances our position in the input string.
// It also updates the line and column count.
func (l *Lexer) readChar() {
	// Before advancing, check if the current character was a newline
	if l.ch == '\n' {
		l.line++
		l.column = 0 // Reset column, it will be incremented below
	}

	if l.readPosition >= len(l.input) {
		l.ch = 0 // 0 is ASCII for NUL, signifies EOF
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
	l.column++ // Increment column for the character now at l.position
}

// peekChar looks ahead in the input without consuming the character.
func (l *Lexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0
	} else {
		return l.input[l.readPosition]
	}
}

// skipWhitespace consumes whitespace characters (space, tab, newline, carriage return, and Unicode whitespace).
// It relies on readChar to update line and column counts.
func (l *Lexer) skipWhitespace() {
	for {
		// ASCII whitespace and control characters (fast path)
		if l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' ||
			l.ch == '\f' || l.ch == '\v' { // Form feed and vertical tab
			l.readChar()
			continue
		}

		// Unicode whitespace characters
		if l.ch >= 128 {
			// Read UTF-8 rune to check if it's Unicode whitespace
			remaining := []byte(l.input[l.position:])
			r, size := utf8.DecodeRune(remaining)
			if r != utf8.RuneError && isUnicodeWhitespace(r) {
				// Skip the multi-byte Unicode whitespace character
				for i := 0; i < size; i++ {
					l.readChar()
				}
				continue
			} else if r != utf8.RuneError && isInvalidInTokenStream(r) {
				// This is a Format control character that should not appear in token stream
				// Don't skip it - let the main tokenization handle it as illegal token
				break
			}
		}

		// Not whitespace, stop skipping
		break
	}
}

// skipWhitespaceAndPeek skips whitespace and returns the next non-whitespace character
// without consuming any characters - just looks ahead
func (l *Lexer) skipWhitespaceAndPeek() byte {
	savedPos := l.position
	savedReadPos := l.readPosition
	savedLine := l.line
	savedColumn := l.column
	savedCh := l.ch

	// First advance to the next character
	l.readChar()

	// Then skip any whitespace
	l.skipWhitespace()

	nextChar := l.ch

	// Restore position
	l.position = savedPos
	l.readPosition = savedReadPos
	l.line = savedLine
	l.column = savedColumn
	l.ch = savedCh // Restore the original character

	return nextChar
}

// PreprocessUnicodeEscapesContextAware replaces unicode escape sequences with actual characters,
// but only when NOT inside string literals, comments, or other contexts where they should remain as escapes
func PreprocessUnicodeEscapesContextAware(input string) string {
	result := strings.Builder{}
	i := 0
	inString := false
	stringChar := byte(0)
	inLineComment := false
	inBlockComment := false

	for i < len(input) {
		if inLineComment {
			// Inside line comment - copy everything until end of line
			if input[i] == '\n' {
				inLineComment = false
			}
			result.WriteByte(input[i])
			i++
			continue
		}

		if inBlockComment {
			// Inside block comment - copy everything until */
			if input[i] == '*' && i+1 < len(input) && input[i+1] == '/' {
				inBlockComment = false
				result.WriteByte(input[i])
				result.WriteByte(input[i+1])
				i += 2
				continue
			}
			result.WriteByte(input[i])
			i++
			continue
		}

		if inString {
			// Inside string literal
			if input[i] == stringChar {
				// End of string
				inString = false
				stringChar = 0
				result.WriteByte(input[i])
				i++
				continue
			} else if input[i] == '\\' && i+1 < len(input) {
				// Escape sequence inside string - keep as literal escape
				result.WriteByte(input[i])
				result.WriteByte(input[i+1])
				i += 2
				continue
			} else {
				// Regular character inside string
				result.WriteByte(input[i])
				i++
				continue
			}
		}

		// Not in any special context - check for string start
		if input[i] == '"' || input[i] == '\'' || input[i] == '`' {
			inString = true
			stringChar = input[i]
			result.WriteByte(input[i])
			i++
			continue
		}

		// Check for comments
		if input[i] == '/' && i+1 < len(input) {
			if input[i+1] == '/' {
				// Line comment start
				inLineComment = true
				result.WriteByte(input[i])
				result.WriteByte(input[i+1])
				i += 2
				continue
			} else if input[i+1] == '*' {
				// Block comment start
				inBlockComment = true
				result.WriteByte(input[i])
				result.WriteByte(input[i+1])
				i += 2
				continue
			}
		}

		// Process Unicode escapes in code context (not in strings/comments)
		if input[i] == '\\' && i+1 < len(input) && input[i+1] == 'u' {
			// Found unicode escape sequence
			i++ // Skip '\'
			i++ // Skip 'u'

			var unicodeHex string
			var isBraced bool

			if i < len(input) && input[i] == '{' {
				// \u{XXXX} format
				i++ // Skip '{'
				for i < len(input) && input[i] != '}' && isHexDigit(input[i]) {
					unicodeHex += string(input[i])
					i++
				}
				if i < len(input) && input[i] == '}' {
					i++ // Skip '}'
					isBraced = true
				}
			} else {
				// \uXXXX format
				for j := 0; j < 4 && i < len(input) && isHexDigit(input[i]); j++ {
					unicodeHex += string(input[i])
					i++
				}
			}

			// Convert hex to character
			if len(unicodeHex) > 0 {
				if codePoint, err := strconv.ParseInt(unicodeHex, 16, 32); err == nil {
					result.WriteRune(rune(codePoint))
				} else {
					// Invalid hex, keep original
					result.WriteString("\\u")
					if isBraced {
						result.WriteByte('{')
						result.WriteString(unicodeHex)
						result.WriteByte('}')
					} else {
						result.WriteString(unicodeHex)
					}
				}
			} else {
				// No hex digits found, keep original
				result.WriteString("\\u")
				if isBraced {
					result.WriteByte('{')
				}
			}
		} else {
			// Regular character
			result.WriteByte(input[i])
			i++
		}
	}

	return result.String()
}

// isWhitespaceUnicodeEscape checks if the current position contains a Unicode escape
// sequence that resolves to a whitespace character
func (l *Lexer) isWhitespaceUnicodeEscape() bool {
	// Save current position
	savedPos := l.readPosition
	savedLine := l.line
	savedColumn := l.column

	// Skip '\u' part
	l.readChar() // Skip '\'
	l.readChar() // Skip 'u'

	var unicodeHex string
	if l.ch == '{' {
		// \u{XXXX} format
		l.readChar() // Skip '{'
		for l.ch != 0 && l.ch != '}' && isHexDigit(l.ch) {
			unicodeHex += string(l.ch)
			l.readChar()
		}
		if l.ch == '}' {
			l.readChar() // Skip '}'
		}
	} else {
		// \uXXXX format
		for i := 0; i < 4 && isHexDigit(l.ch); i++ {
			unicodeHex += string(l.ch)
			l.readChar()
		}
	}

	// Check if the hex value is a whitespace character
	if len(unicodeHex) > 0 {
		if codePoint, err := strconv.ParseInt(unicodeHex, 16, 32); err == nil {
			r := rune(codePoint)
			if isUnicodeWhitespace(r) {
				// Restore position and return true
				l.readPosition = savedPos
				l.line = savedLine
				l.column = savedColumn
				l.ch = l.input[savedPos-1]
				return true
			}
		}
	}

	// Restore position and return false
	l.readPosition = savedPos
	l.line = savedLine
	l.column = savedColumn
	l.ch = l.input[savedPos-1]
	return false
}

// isUnicodeWhitespace checks if a rune is considered whitespace in JavaScript/ECMAScript
func isUnicodeWhitespace(r rune) bool {
	// JavaScript whitespace characters according to ECMAScript specification
	switch r {
	case 0x0009, // Tab
		0x000B,                                                                                 // Vertical Tab
		0x000C,                                                                                 // Form Feed
		0x0020,                                                                                 // Space
		0x00A0,                                                                                 // Non-breaking space
		0x1680,                                                                                 // Ogham space mark
		0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006, 0x2007, 0x2008, 0x2009, 0x200A, // Various Unicode spaces
		0x2028, // Line separator
		0x2029, // Paragraph separator
		0x202F, // Narrow no-break space
		0x205F, // Medium mathematical space
		0x3000, // Ideographic space
		0xFEFF: // Zero width no-break space (BOM)
		return true
	}
	return false
}

// isFormatControlCharacter checks if a rune is a Format control character that should not appear in token streams
func isFormatControlCharacter(r rune) bool {
	// Format control characters that should not appear in unexpected positions
	switch r {
	case 0x180E: // Mongolian Vowel Separator (U+180E) - should cause SyntaxError
		return true
	}
	return false
}

// isInvalidInTokenStream checks if a Unicode character is invalid in the token stream
func isInvalidInTokenStream(r rune) bool {
	// Characters that should not appear in the token stream and should cause SyntaxError
	if isFormatControlCharacter(r) {
		return true
	}
	// For debugging - log all Unicode characters >= 128
	if r >= 128 {
		debugPrintf("isInvalidInTokenStream: checking character U+%04X (%c)", r, r)
	}
	return false
}

// NextToken scans the input and returns the next token.
func (l *Lexer) NextToken() Token {
	debugPrintf("NextToken: pos=%d ch='%c' tmpl=%v", l.position, l.ch, l.inTemplate)
	// --- NEW: Check pushback buffer first ---
	if l.pushedToken != nil {
		tok := *l.pushedToken
		l.pushedToken = nil
		debugPrintf("NextToken: PUSHBACK - returning pushed token %s, ch='%c', position=%d", tok.Type, l.ch, l.position)

		// Update previous token for regex context determination
		l.prevToken = tok.Type

		return tok
	}

	var tok Token

	// --- MODIFIED: Don't skip whitespace inside template literals ---
	if !l.inTemplate || l.braceDepth > 0 {
		l.skipWhitespace()
	}

	// Capture token start position *after* skipping whitespace
	startLine := l.line
	startCol := l.column
	startPos := l.position

	// --- NEW: Check for invalid Unicode characters in token stream ---
	if l.ch >= 128 {
		remaining := []byte(l.input[l.position:])
		r, size := utf8.DecodeRune(remaining)
		if r != utf8.RuneError && isInvalidInTokenStream(r) {
			// This is a Format control character that should not appear in token stream
			illegalLiteral := string(r)
			illegalToken := Token{
				Type:     ILLEGAL,
				Literal:  illegalLiteral,
				Line:     startLine,
				Column:   startCol,
				StartPos: startPos,
				EndPos:   startPos + size,
			}
			// Skip the invalid character
			for i := 0; i < size; i++ {
				l.readChar()
			}
			return illegalToken
		}
	}

	// --- NEW: Handle template literal state ---
	if l.inTemplate && l.braceDepth == 0 {
		// We're inside a template literal but not in an interpolation
		// Check if we're at the start of an interpolation or template string
		if l.ch == '$' && l.peekChar() == '{' {
			// Start of interpolation: ${
			literal := l.input[startPos : l.position+2] // "${"
			l.readChar()                                // Consume '$'
			l.readChar()                                // Consume '{'
			l.braceDepth = 1                            // Start tracking braces
			return Token{
				Type:     TEMPLATE_INTERPOLATION,
				Literal:  literal,
				Line:     startLine,
				Column:   startCol,
				StartPos: startPos,
				EndPos:   l.position,
			}
		} else if l.ch == '`' {
			// End of template literal - handle it directly here
			debugPrintf("NextToken: TEMPLATE MODE - calling readTemplateLiteral for closing backtick, ch='%c', position=%d", l.ch, l.position)
			result := l.readTemplateLiteral(startLine, startCol, startPos)
			debugPrintf("NextToken: TEMPLATE MODE - got result from readTemplateLiteral, result=%s, ch='%c', position=%d", result.Type, l.ch, l.position)
			debugPrintf("NextToken: TEMPLATE MODE - about to return result")
			debugPrintf("NextToken: TEMPLATE MODE - RETURNING NOW")
			return result
		} else {
			// Read template string content
			return l.readTemplateString(startLine, startCol, startPos)
		}
	}

	// --- MODIFIED: Handle closing braces in interpolation mode ---
	if l.inTemplate && l.braceDepth > 0 && l.ch == '}' {
		l.braceDepth--
		if l.braceDepth == 0 {
			// End of interpolation, back to template string mode
			literal := string(l.ch)
			l.readChar()
			return Token{
				Type:     RBRACE,
				Literal:  literal,
				Line:     startLine,
				Column:   startCol,
				StartPos: startPos,
				EndPos:   l.position,
			}
		}
		// Fall through to normal brace handling
	}

	// --- MODIFIED: Track opening braces in interpolation mode ---
	if l.inTemplate && l.braceDepth > 0 && l.ch == '{' {
		l.braceDepth++
		// Fall through to normal brace handling
	}

	switch l.ch {
	case '=':
		if l.peekChar() == '=' {
			l.readChar() // Consume '='
			if l.peekChar() == '=' {
				l.readChar()                                // Consume second '='
				literal := l.input[startPos : l.position+1] // Read the actual '==='
				l.readChar()                                // Advance past '='
				tok = Token{Type: STRICT_EQ, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			} else {
				literal := l.input[startPos : l.position+1] // Read the actual '=='
				l.readChar()                                // Advance past '='
				tok = Token{Type: EQ, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			}
		} else if l.peekChar() == '>' {
			l.readChar()                                // Consume '>'
			literal := l.input[startPos : l.position+1] // Read the actual '=>'
			l.readChar()                                // Advance past '>'
			tok = Token{Type: ARROW, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else {
			literal := string(l.ch) // Just '='
			l.readChar()            // Advance past '='
			tok = Token{Type: ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '!':
		if l.peekChar() == '=' {
			l.readChar() // Consume '='
			if l.peekChar() == '=' {
				l.readChar()                                // Consume second '='
				literal := l.input[startPos : l.position+1] // Read the actual '!=='
				l.readChar()                                // Advance past '='
				tok = Token{Type: STRICT_NOT_EQ, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			} else {
				literal := l.input[startPos : l.position+1] // Read the actual '!='
				l.readChar()                                // Advance past '='
				tok = Token{Type: NOT_EQ, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			}
		} else {
			literal := string(l.ch) // Just '!'
			l.readChar()            // Advance past '!'
			tok = Token{Type: BANG, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '+':
		if l.peekChar() == '=' { // Check for +=
			l.readChar()                                // Consume '='
			literal := l.input[startPos : l.position+1] // Read the actual '+='
			l.readChar()                                // Advance past '='
			tok = Token{Type: PLUS_ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else if l.peekChar() == '+' { // Check for ++
			l.readChar()                                // Consume second '+'
			literal := l.input[startPos : l.position+1] // Read the actual '++'
			l.readChar()                                // Advance past '+'
			tok = Token{Type: INC, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else {
			literal := string(l.ch) // Just '+'
			l.readChar()            // Advance past '+'
			tok = Token{Type: PLUS, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '-':
		if l.peekChar() == '=' { // Check for -=
			l.readChar()                                // Consume '='
			literal := l.input[startPos : l.position+1] // Read the actual '-='
			l.readChar()                                // Advance past '='
			tok = Token{Type: MINUS_ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else if l.peekChar() == '-' { // Check for --
			l.readChar()                                // Consume second '-'
			literal := l.input[startPos : l.position+1] // Read the actual '--'
			l.readChar()                                // Advance past '-'
			tok = Token{Type: DEC, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else {
			literal := string(l.ch) // Just '-'
			l.readChar()            // Advance past '-'
			tok = Token{Type: MINUS, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '*':
		if l.peekChar() == '*' { // Potential ** or **=
			// Look two chars ahead for '='
			secondCharPos := l.readPosition + 1
			var thirdChar byte = 0
			if secondCharPos < len(l.input) {
				thirdChar = l.input[secondCharPos]
			}

			if thirdChar == '=' { // Check for **=
				l.readChar()                                // Consume second *
				l.readChar()                                // Consume =
				literal := l.input[startPos : l.position+1] // Read "**="
				l.readChar()                                // Advance past =
				tok = Token{Type: EXPONENT_ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			} else { // Just **
				l.readChar()                                // Consume second *
				literal := l.input[startPos : l.position+1] // Read "**"
				l.readChar()                                // Advance past second *
				tok = Token{Type: EXPONENT, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			}
		} else if l.peekChar() == '=' { // Check for *=
			l.readChar()                                // Consume =
			literal := l.input[startPos : l.position+1] // Read "*="
			l.readChar()                                // Advance past =
			tok = Token{Type: ASTERISK_ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else { // Just *
			literal := string(l.ch) // Read "*"
			l.readChar()            // Advance past *
			tok = Token{Type: ASTERISK, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '/':
		if l.peekChar() == '/' {
			l.skipComment()      // Skips to the end of the line or EOF
			return l.NextToken() // Recursively call NextToken to get the token after the comment
		} else if l.peekChar() == '*' {
			if !l.skipMultilineComment() { // Skips until '*/' or EOF
				// Unterminated comment, return an ILLEGAL token
				literal := "Unterminated multiline comment"
				// Use start position of the '/*' but the error ends at current position (EOF)
				tok = Token{Type: ILLEGAL, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
				return tok // Explicitly return, don't advance char
			}
			return l.NextToken() // Get the token after the multiline comment
		} else if l.peekChar() == '=' { // Added check for /=
			l.readChar()                                // Consume '='
			literal := l.input[startPos : l.position+1] // Read the actual '/='
			l.readChar()                                // Advance past '='
			tok = Token{Type: SLASH_ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else if canBeRegexStart(l.prevToken) {
			// DEBUG: Add debug output for regex context
			debugPrintf("Attempting regex parse: prevToken=%s, position=%d", l.prevToken, l.position)
			// Try to read as regex literal
			pattern, flags, success, foundComplete := l.readRegexLiteral()
			if success {
				// Successfully read regex literal - skip any whitespace that follows
				l.skipWhitespace()
				literal := "/" + pattern + "/" + flags
				tok = Token{Type: REGEX_LITERAL, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			} else if foundComplete {
				// Found a complete regex pattern but with invalid flags/format
				// This is an error, not a division operator
				literal := "Invalid regex literal"
				tok = Token{Type: ILLEGAL, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			} else {
				// Failed to find complete regex pattern - backtrack and treat as division
				// readRegexLiteral already restored the lexer state
				literal := string(l.ch) // Just '/'
				l.readChar()            // Advance past '/'
				tok = Token{Type: SLASH, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			}
		} else {
			// Context doesn't allow regex, treat as division operator
			literal := string(l.ch) // Just '/'
			l.readChar()            // Advance past '/'
			tok = Token{Type: SLASH, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '&':
		peek := l.peekChar()
		if peek == '&' { // Logical AND or Logical AND Assignment
			l.readChar()             // Consume second '&'
			if l.peekChar() == '=' { // Check for &&=
				l.readChar()                                // Consume '='
				literal := l.input[startPos : l.position+1] // Read "&&="
				l.readChar()                                // Advance past '='
				tok = Token{Type: LOGICAL_AND_ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			} else { // Just &&
				literal := l.input[startPos : l.position+1] // Read "&&"
				l.readChar()                                // Advance past second '&'
				tok = Token{Type: LOGICAL_AND, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			}
		} else if peek == '=' { // Bitwise AND Assignment &=
			l.readChar()                                // Consume '='
			literal := l.input[startPos : l.position+1] // Read "&="
			l.readChar()                                // Advance past '='
			tok = Token{Type: BITWISE_AND_ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else { // Bitwise AND &
			literal := string(l.ch) // Read "&"
			l.readChar()            // Advance past '&'
			tok = Token{Type: BITWISE_AND, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '|':
		peek := l.peekChar()
		if peek == '|' { // Logical OR or Logical OR Assignment
			l.readChar()             // Consume second '|'
			if l.peekChar() == '=' { // Check for ||=
				l.readChar()                                // Consume '='
				literal := l.input[startPos : l.position+1] // Read "||="
				l.readChar()                                // Advance past '='
				tok = Token{Type: LOGICAL_OR_ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			} else { // Just ||
				literal := l.input[startPos : l.position+1] // Read "||"
				l.readChar()                                // Advance past second '|'
				tok = Token{Type: LOGICAL_OR, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			}
		} else if peek == '=' { // Bitwise OR Assignment |=
			l.readChar()                                // Consume '='
			literal := l.input[startPos : l.position+1] // Read "|="
			l.readChar()                                // Advance past '='
			tok = Token{Type: BITWISE_OR_ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else { // Single '|' is Union Type / Bitwise OR (Let's prioritize Union for now, maybe reconsider if needed)
			literal := string(l.ch)
			l.readChar()
			// For now, assume PIPE for type context. If needed later, parser context can disambiguate.
			tok = Token{Type: PIPE, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			// Alternative: tok = Token{Type: BITWISE_OR, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '^':
		if l.peekChar() == '=' { // Bitwise XOR Assignment ^=
			l.readChar()                                // Consume '='
			literal := l.input[startPos : l.position+1] // Read "^="
			l.readChar()                                // Advance past '='
			tok = Token{Type: BITWISE_XOR_ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else { // Bitwise XOR ^
			literal := string(l.ch) // Read "^"
			l.readChar()            // Advance past '^'
			tok = Token{Type: BITWISE_XOR, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '~': // Bitwise NOT ~
		literal := string(l.ch) // Read "~"
		l.readChar()            // Advance past '~'
		tok = Token{Type: BITWISE_NOT, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}

	case '<':
		// Check if next non-whitespace character is '=' for <=
		peekAfterWS := l.skipWhitespaceAndPeek()
		if peekAfterWS == '=' { // Less than or equal <=
			l.skipWhitespace()                          // Actually consume the whitespace
			l.readChar()                                // Consume '='
			literal := l.input[startPos : l.position+1] // Read the actual '<='
			l.readChar()                                // Advance past '='
			tok = Token{Type: LE, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else if peekAfterWS == '<' { // Left shift << or Left shift assignment <<=
			l.skipWhitespace() // Actually consume the whitespace
			l.readChar()       // Consume second '<'
			peek2AfterWS := l.skipWhitespaceAndPeek()
			if peek2AfterWS == '=' { // Check for <<=
				l.skipWhitespace()                          // Actually consume the whitespace
				l.readChar()                                // Consume '='
				literal := l.input[startPos : l.position+1] // Read "<<="
				l.readChar()                                // Advance past '='
				tok = Token{Type: LEFT_SHIFT_ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			} else { // Just <<
				literal := l.input[startPos : l.position+1] // Read "<<"
				l.readChar()                                // Advance past second '<'
				tok = Token{Type: LEFT_SHIFT, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			}
		} else { // Just Less than <
			literal := string(l.ch) // Just '<'
			l.readChar()            // Advance past '<'
			tok = Token{Type: LT, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '>':
		// Check for >= without whitespace (ECMAScript requires no whitespace in compound tokens)
		if l.peekChar() == '=' { // Greater than or equal >=
			l.readChar()                                // Consume '='
			literal := l.input[startPos : l.position+1] // Read the actual '>='
			l.readChar()                                // Advance past '='
			tok = Token{Type: GE, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else {
			// Check for >> operators (allow whitespace for TypeScript generic syntax)
			peekAfterWS := l.skipWhitespaceAndPeek()
			if peekAfterWS == '>' { // Right shift >>, Unsigned right shift >>>, or assignments
			l.skipWhitespace() // Actually consume the whitespace
			l.readChar()       // Consume second '>'
			peek2AfterWS := l.skipWhitespaceAndPeek()
			if peek2AfterWS == '>' { // Potential >>> or >>>=
				l.skipWhitespace() // Actually consume the whitespace
				l.readChar()       // Consume third '>'
				peek3AfterWS := l.skipWhitespaceAndPeek()
				if peek3AfterWS == '=' { // Check for >>>=
					l.skipWhitespace()                                             // Actually consume the whitespace
					l.readChar()                                                   // Consume '='
					literal := strings.TrimSpace(l.input[startPos : l.position+1]) // Read ">>>="
					l.readChar()                                                   // Advance past '='
					tok = Token{Type: UNSIGNED_RIGHT_SHIFT_ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
				} else { // Just >>>
					literal := strings.TrimSpace(l.input[startPos : l.position+1]) // Read ">>>"
					l.readChar()                                                   // Advance past third '>'
					tok = Token{Type: UNSIGNED_RIGHT_SHIFT, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
				}
			} else if peek2AfterWS == '=' { // Check for >>=
				l.skipWhitespace()                                             // Actually consume the whitespace
				l.readChar()                                                   // Consume '='
				literal := strings.TrimSpace(l.input[startPos : l.position+1]) // Read ">>="
				l.readChar()                                                   // Advance past '='
				tok = Token{Type: RIGHT_SHIFT_ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			} else { // Just >>
				literal := strings.TrimSpace(l.input[startPos : l.position+1]) // Read ">>"
				l.readChar()                                                   // Advance past second '>'
				tok = Token{Type: RIGHT_SHIFT, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			}
			} else { // Just Greater than >
				literal := string(l.ch) // Just '>'
				l.readChar()            // Advance past '>'
				tok = Token{Type: GT, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			}
		}
	case ';':
		literal := string(l.ch)
		l.readChar()
		tok = Token{Type: SEMICOLON, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
	case ':':
		literal := string(l.ch)
		l.readChar()
		tok = Token{Type: COLON, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
	case ',':
		literal := string(l.ch)
		l.readChar()
		tok = Token{Type: COMMA, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
	case '(':
		literal := string(l.ch)
		l.readChar()
		tok = Token{Type: LPAREN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
	case ')':
		literal := string(l.ch)
		l.readChar()
		tok = Token{Type: RPAREN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
	case '{':
		literal := string(l.ch)
		l.readChar()
		tok = Token{Type: LBRACE, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
	case '}':
		literal := string(l.ch)
		l.readChar()
		tok = Token{Type: RBRACE, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
	case '[':
		literal := string(l.ch)
		l.readChar()
		tok = Token{Type: LBRACKET, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
	case ']':
		literal := string(l.ch)
		l.readChar()
		tok = Token{Type: RBRACKET, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
	case '"': // Double quoted string
		literal, ok := l.readString('"')
		endPos := l.position // readString advances past the closing quote if successful
		if !ok {
			// Determine if it was unterminated or invalid escape
			// For now, use a generic message. l.position is where the error occurred.
			tok = Token{Type: ILLEGAL, Literal: "Invalid string literal", Line: startLine, Column: startCol, StartPos: startPos, EndPos: endPos}
		} else {
			tok = Token{Type: STRING, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: endPos}
		}
	case '\'': // Single quoted string
		literal, ok := l.readString('\'')
		endPos := l.position // readString advances past the closing quote if successful
		if !ok {
			tok = Token{Type: ILLEGAL, Literal: "Invalid string literal", Line: startLine, Column: startCol, StartPos: startPos, EndPos: endPos}
		} else {
			tok = Token{Type: STRING, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: endPos}
		}
	case '?':
		peek := l.peekChar()
		if peek == '?' { // Nullish Coalescing ?? or Assignment ??=
			l.readChar()             // Consume second '?'
			if l.peekChar() == '=' { // Check for ??=
				l.readChar()                                // Consume '='
				literal := l.input[startPos : l.position+1] // Read "??="
				l.readChar()                                // Advance past '='
				tok = Token{Type: COALESCE_ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			} else { // Just ??
				literal := l.input[startPos : l.position+1] // Read "??"
				l.readChar()                                // Advance past second '?'
				tok = Token{Type: COALESCE, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			}
		} else if peek == '.' { // Optional Chaining ?.
			l.readChar()                                // Consume '.'
			literal := l.input[startPos : l.position+1] // Read "?."
			l.readChar()                                // Advance past '.'
			tok = Token{Type: OPTIONAL_CHAINING, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else { // Original ternary operator ?
			literal := string(l.ch)
			l.readChar()
			tok = Token{Type: QUESTION, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '.':
		if l.peekChar() == '.' {
			l.readChar() // Consume second '.'
			if l.peekChar() == '.' {
				l.readChar()                                // Consume third '.'
				literal := l.input[startPos : l.position+1] // Read the actual '...'
				l.readChar()                                // Advance past '.'
				tok = Token{Type: SPREAD, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			} else {
				// Sequence like '..' is illegal. Treat the first dot as DOT.
				// We already consumed the second dot, so don't need SetPosition.
				// We just need to create the token for the *first* dot.
				literal := string(l.input[startPos])
				// Reset lexer to be positioned *after* the first dot for the next token
				l.SetPosition(startPos + 1) // This resets l.ch, l.position, l.readPosition, but NOT line/col
				// Manually fix column as SetPosition doesn't handle it well
				// We know we are on the same line, just one char after the start.
				l.line = startLine
				l.column = startCol + 1
				tok = Token{Type: DOT, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: startPos + 1}
			}
		} else {
			// Just a single dot
			literal := string(l.ch)
			l.readChar()
			tok = Token{Type: DOT, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '%':
		if l.peekChar() == '=' { // Check for %=
			l.readChar()                                // Consume '='
			literal := l.input[startPos : l.position+1] // Read "%="
			l.readChar()                                // Advance past '='
			tok = Token{Type: REMAINDER_ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else {
			literal := string(l.ch) // Just '%'
			l.readChar()            // Advance past '%'
			tok = Token{Type: REMAINDER, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '`': // Template literal backtick
		debugPrintf("NextToken: SWITCH CASE ` - calling readTemplateLiteral, ch='%c', position=%d", l.ch, l.position)
		result := l.readTemplateLiteral(startLine, startCol, startPos)
		debugPrintf("NextToken: SWITCH CASE ` - got result from readTemplateLiteral, result=%s, ch='%c', position=%d", result.Type, l.ch, l.position)
		debugPrintf("NextToken: SWITCH CASE ` - about to return result")
		return result
	case 0: // EOF
		tok = Token{Type: EOF, Literal: "", Line: startLine, Column: startCol, StartPos: startPos, EndPos: startPos}
	default:
		if l.canStartIdentifier() {
			literal := l.readIdentifierWithUnicode() // Consumes letters/digits/_/$/unicode escapes/unicode chars
			tokType := LookupIdent(literal)
			// readIdentifierWithUnicode leaves l.position *after* the last char of the identifier
			tok = Token{Type: tokType, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			//return tok // Return early, readIdentifierWithUnicode already called readChar()
		} else if isDigit(l.ch) {
			literal := l.readNumber() // Consumes digits and potentially '.'
			// readNumber leaves l.position *after* the last char of the number

			// Check for BigInt suffix 'n'
			if l.ch == 'n' {
				l.readChar() // Consume the 'n' suffix
				tok = Token{Type: BIGINT, Literal: literal + "n", Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
				// Debug: Check if this looks like it might be mis-tokenized
				if strings.Contains(literal, ".") {
					// This should not happen for valid BigInt literals
					fmt.Printf("DEBUG: Lexer produced BIGINT token with literal: %q\n", literal)
				}
			} else {
				tok = Token{Type: NUMBER, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			}
			//return tok // Return early, readNumber already called readChar()
		} else if l.ch == '#' {
			if l.peekChar() == '!' && l.line == 1 && (l.position == 0 || (l.position == 1 && startPos == 0)) {
				// Hashbang comment - only valid at very start of file
				l.skipHashbangComment()
				l.skipWhitespace()   // Skip any whitespace after the hashbang comment
				return l.NextToken() // Get the next token after the comment
			} else {
				// Try to read a private identifier: #identifier (including Unicode escapes)
				l.readChar() // Consume '#'

				// Save position in case we need to backtrack
				savedPosition := l.position
				savedCh := l.ch

				// Try to read an identifier (including Unicode escapes)
				identifierPart := l.readIdentifierWithUnicode()

				if identifierPart != "" {
					// Successfully read an identifier part
					literal := "#" + identifierPart
					tok = Token{Type: PRIVATE_IDENT, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
				} else {
					// No valid identifier after '#' - restore position and treat as illegal
					l.position = savedPosition
					l.ch = savedCh
					literal := string('#')
					tok = Token{Type: ILLEGAL, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
				}
			}
		} else {
			// Illegal character
			literal := string(l.ch)
			l.readChar() // Consume the illegal character
			tok = Token{Type: ILLEGAL, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	}

	debugPrintf("NextToken: END - returning tok=%s, literal=%s", tok.Type, tok.Literal)
	debugPrintf("Token: %s, %s, %d, %d, %d, %d", tok.Type, tok.Literal, tok.Line, tok.Column, tok.StartPos, tok.EndPos)

	// Update previous token for regex context determination
	l.prevToken = tok.Type

	return tok
}

// readIdentifier reads an identifier (letters, digits, _) and advances the lexer's position.
// It returns the literal string found.
func (l *Lexer) readIdentifier() string {
	startPos := l.position
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' || l.ch == '$' {
		l.readChar()
	}
	return l.input[startPos:l.position]
}

// readIdentifierWithUnicode reads an identifier that may contain unicode escape sequences
// or Unicode characters, and returns the resolved identifier string (e.g., "\u0064o" becomes "do")
func (l *Lexer) readIdentifierWithUnicode() string {
	var result strings.Builder

	for l.ch != 0 {
		if l.ch == '\\' && l.peekChar() == 'u' {
			// Parse unicode escape sequence \uXXXX or \u{...}
			l.readChar() // consume '\'
			l.readChar() // consume 'u'

			var unicodeHex string
			var validUnicode bool

			if l.ch == '{' {
				// \u{XXXX} format - variable length
				l.readChar() // consume '{'
				for l.ch != 0 && l.ch != '}' && isHexDigit(l.ch) {
					unicodeHex += string(l.ch)
					l.readChar()
				}
				if l.ch == '}' {
					l.readChar() // consume '}'
					validUnicode = len(unicodeHex) > 0
				}
			} else {
				// \uXXXX format - exactly 4 hex digits
				for i := 0; i < 4; i++ {
					if l.ch != 0 && isHexDigit(l.ch) {
						unicodeHex += string(l.ch)
						l.readChar()
					} else {
						break
					}
				}
				validUnicode = len(unicodeHex) == 4
			}

			if validUnicode {
				// Convert hex to rune
				if codePoint, err := strconv.ParseInt(unicodeHex, 16, 32); err == nil {
					r := rune(codePoint)
					// Check if the Unicode character is valid for identifiers
					if result.Len() == 0 {
						// First character - must be ID_Start
						if isUnicodeIDStart(r) {
							result.WriteRune(r)
							continue
						} else {
							// Invalid start character - fall back to literal
							result.WriteString("\\u")
							if unicodeHex != "" {
								result.WriteString(unicodeHex)
							}
							break
						}
					} else {
						// Continuation character - must be ID_Continue
						if isUnicodeIDContinue(r) {
							result.WriteRune(r)
							continue
						} else {
							// Invalid continue character - stop here
							// Don't consume this character, backtrack
							break
						}
					}
				} else {
					// Invalid hex - fall back to literal
					result.WriteString("\\u")
					result.WriteString(unicodeHex)
					break
				}
			} else {
				// Invalid unicode escape - fall back to literal
				result.WriteString("\\u")
				if unicodeHex != "" {
					result.WriteString(unicodeHex)
				}
				break
			}
		} else {
			// Check if current position starts a Unicode character
			if l.ch >= 128 { // Non-ASCII character
				// Read UTF-8 rune
				remaining := []byte(l.input[l.position:])
				r, size := utf8.DecodeRune(remaining)
				if r == utf8.RuneError {
					// Invalid UTF-8, treat as illegal
					break
				}

				// Check if this Unicode character is valid for identifiers
				if result.Len() == 0 {
					// First character - must be ID_Start
					if isUnicodeIDStart(r) {
						result.WriteRune(r)
						// Advance position by the UTF-8 character size
						for i := 0; i < size; i++ {
							l.readChar()
						}
					} else {
						// Not a valid identifier start character
						break
					}
				} else {
					// Continuation character - must be ID_Continue
					if isUnicodeIDContinue(r) {
						result.WriteRune(r)
						// Advance position by the UTF-8 character size
						for i := 0; i < size; i++ {
							l.readChar()
						}
					} else {
						// Not a valid identifier continue character
						break
					}
				}
			} else {
				// ASCII character - use existing logic
				if result.Len() == 0 {
					// First character
					if isLetter(l.ch) || l.ch == '$' {
						result.WriteByte(l.ch)
						l.readChar()
					} else {
						break
					}
				} else {
					// Continuation character
					if isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' || l.ch == '$' {
						result.WriteByte(l.ch)
						l.readChar()
					} else if l.ch == '\\' && l.peekChar() == 'u' {
						// Unicode escape sequence - continue to top of loop to handle it
						continue
					} else {
						break
					}
				}
			}
		}
	}

	return result.String()
}

// readNumber reads a number literal (integer or float, various bases) and advances the lexer's position.
// Handles decimal (optional exponent/fraction), hex (0x), binary (0b), octal (0o).
// Handles numeric separators '_'.
// Returns the raw literal string found.
// It performs basic validation (e.g., separator placement) and stops if invalid sequence is found.
func (l *Lexer) readNumber() string {
	startPos := l.position
	base := 10
	consumedPrefix := false

	// 1. Check for base prefix (0x, 0b, 0o)
	if l.ch == '0' {
		peek := l.peekChar()
		switch peek {
		case 'x', 'X':
			base = 16
			l.readChar() // Consume '0'
			l.readChar() // Consume 'x' or 'X'
			consumedPrefix = true
		case 'b', 'B':
			base = 2
			l.readChar() // Consume '0'
			l.readChar() // Consume 'b' or 'B'
			consumedPrefix = true
		case 'o', 'O':
			base = 8
			l.readChar() // Consume '0'
			l.readChar() // Consume 'o' or 'O'
			consumedPrefix = true
		}
	}

	// 2. Read integer part (handling separators)
	lastCharWasDigit := false
	for {
		if isDigitForBase(l.ch, base) {
			l.readChar()
			lastCharWasDigit = true
		} else if l.ch == '_' {
			if !lastCharWasDigit { // Separator must follow a digit
				// Invalid format (e.g., 0x_1, 1__2, starts with _)
				// Return what we have *before* the invalid separator.
				return l.input[startPos:l.position]
			}
			l.readChar()                     // Consume '_'
			if !isDigitForBase(l.ch, base) { // Separator must be followed by a digit
				// Invalid format (e.g., 1_)
				// Return what we have *before* the separator and the following non-digit.
				return l.input[startPos : l.position-1]
			}
			lastCharWasDigit = false // Reset after consuming separator
		} else {
			break // Not a valid digit or separator for this base
		}
	}

	// Check if *any* digits were read after the prefix
	if consumedPrefix && l.position == startPos+2 {
		// Only prefix was read (e.g., "0x", "0b") - invalid
		// Return just the prefix as the consumed part.
		return l.input[startPos:l.position]
	}

	// 3. Read fractional part (only for base 10)
	if base == 10 && l.ch == '.' {
		// Check if the character *after* the dot is a digit or separator
		peek := l.peekChar()
		if isDigit(peek) || peek == '_' {
			l.readChar()             // Consume '.'
			lastCharWasDigit = false // Reset for fraction part validation
			for {
				if isDigit(l.ch) {
					l.readChar()
					lastCharWasDigit = true
				} else if l.ch == '_' {
					if !lastCharWasDigit { // Separator must follow a digit
						return l.input[startPos:l.position]
					}
					l.readChar()        // Consume '_'
					if !isDigit(l.ch) { // Separator must be followed by a digit
						return l.input[startPos : l.position-1]
					}
					lastCharWasDigit = false // Reset
				} else {
					break // End of fractional part
				}
			}
			// Must end fraction with a digit
			if l.input[l.position-1] == '_' {
				return l.input[startPos : l.position-1]
			}
		}
	}

	// 4. Read exponent part (only for base 10)
	if base == 10 && (l.ch == 'e' || l.ch == 'E') {
		l.readChar() // Consume 'e' or 'E'
		if l.ch == '+' || l.ch == '-' {
			l.readChar() // Consume sign
		}

		digitsReadExponent := false
		lastCharWasDigit = false // Reset
		for {
			if isDigit(l.ch) {
				l.readChar()
				lastCharWasDigit = true
				digitsReadExponent = true
			} else if l.ch == '_' {
				if !lastCharWasDigit { // Separator must follow a digit
					return l.input[startPos:l.position]
				}
				l.readChar()        // Consume '_'
				if !isDigit(l.ch) { // Separator must be followed by a digit
					return l.input[startPos : l.position-1]
				}
				lastCharWasDigit = false // Reset
			} else {
				break // End of exponent part
			}
		}

		// Exponent must have digits and not end with separator
		if !digitsReadExponent {
			// Invalid: 'e'/'E' not followed by digits (e.g., "1e", "1e+")
			// Return up to the 'e'/'E' or the sign
			return l.input[startPos:l.position]
		}
		if l.input[l.position-1] == '_' {
			return l.input[startPos : l.position-1]
		}
	}

	return l.input[startPos:l.position]
}

// readString reads a string literal enclosed in the given quote character.
// It handles basic escape sequences: \n, \t, \r, \\, and escaped quotes.
// Returns the unescaped string content and a boolean indicating success.
// Success is false if the string is unterminated or contains an invalid escape sequence.
// Advances the lexer's position to *after* the closing quote if successful.
func (l *Lexer) readString(quote byte) (string, bool) {
	var builder strings.Builder
	// Consume the opening quote
	l.readChar()

	for {
		// Check for termination conditions *before* processing the character
		if l.ch == quote {
			l.readChar() // Consume the closing quote
			return builder.String(), true
		}
		if l.ch == 0 { // EOF
			// Unterminated string
			return "", false
		}

		if l.ch == '\\' { // Handle escape sequence
			l.readChar() // Consume the backslash
			switch l.ch {
			case 'n':
				builder.WriteByte('\n')
			case 't':
				builder.WriteByte('\t')
			case 'r':
				builder.WriteByte('\r')
			case 'f':
				builder.WriteByte('\f') // Form feed (U+000C)
			case 'v':
				builder.WriteByte('\v') // Vertical tab (U+000B)
			case 'b':
				builder.WriteByte('\b') // Backspace (U+0008)
			case 'a':
				builder.WriteByte('\a') // Alert/Bell (U+0007)
			case '0':
				builder.WriteByte('\000') // Null character (U+0000)
			case '\n':
				// Escaped newline: Already consumed by readChar before the switch.
				// Line count was updated. Do nothing else.
			case '\r':
				// Escaped carriage return: Already consumed by readChar before the switch.
				// Check for a subsequent LF in CRLF sequence.
				if l.peekChar() == '\n' {
					l.readChar() // Consume the LF
				}
				// Do nothing else.
			case '\\':
				builder.WriteByte('\\')
			case '/':
				builder.WriteByte('/') // Allow escaped forward slash
			case quote: // Handle escaped quote (' or ")
				builder.WriteByte(quote)
			case 'u':
				// Unicode escape sequence \uXXXX or \u{XXXX}
				if l.peekChar() == '{' {
					// \u{XXXX} format
					l.readChar() // consume '{'
					hexStr := ""
					for l.peekChar() != '}' && l.peekChar() != 0 && len(hexStr) < 6 {
						l.readChar()
						if isHexDigit(l.ch) {
							hexStr += string(l.ch)
						} else {
							return "", false // Invalid hex digit
						}
					}
					if l.peekChar() == '}' {
						l.readChar() // consume '}'
						if codePoint, err := strconv.ParseInt(hexStr, 16, 32); err == nil && codePoint <= 0x10FFFF {
							builder.WriteRune(rune(codePoint))
						} else {
							return "", false // Invalid code point
						}
					} else {
						return "", false // Unterminated \u{...}
					}
				} else {
					// \uXXXX format
					hexStr := ""
					for i := 0; i < 4; i++ {
						if l.peekChar() != 0 && isHexDigit(l.peekChar()) {
							l.readChar()
							hexStr += string(l.ch)
						} else {
							return "", false // Invalid or incomplete \uXXXX
						}
					}
					if codePoint, err := strconv.ParseInt(hexStr, 16, 32); err == nil {
						builder.WriteRune(rune(codePoint))
					} else {
						return "", false // Invalid code point
					}
				}
			case 'x':
				// Hexadecimal escape sequence \xXX
				hexStr := ""
				for i := 0; i < 2; i++ {
					if l.peekChar() != 0 && isHexDigit(l.peekChar()) {
						l.readChar()
						hexStr += string(l.ch)
					} else {
						return "", false // Invalid or incomplete \xXX
					}
				}
				if codePoint, err := strconv.ParseInt(hexStr, 16, 8); err == nil {
					builder.WriteByte(byte(codePoint))
				} else {
					return "", false // Invalid code point
				}
			case 0: // EOF after backslash
				return "", false // Invalid escape sequence due to EOF
			default:
				// Invalid escape sequence (e.g., \z)
				// Option 1: Treat as illegal string
				return "", false
				// Option 2: Treat backslash literally (sometimes allowed)
				// builder.WriteByte('\\')
				// builder.WriteByte(l.ch)
			}
		} else {
			// Regular character
			// Check for unescaped newline within the string, which is often illegal
			if l.ch == '\n' || l.ch == '\r' {
				// Treat unescaped newline as termination error
				return "", false
			}
			builder.WriteByte(l.ch)
		}

		// Advance to the next character *after* processing the current one
		l.readChar()
	}
	// The loop should only be exited via the successful termination check (l.ch == quote)
	// or via returning false for errors. This point should not be reached.
}

// readRegexLiteral reads a regular expression literal /pattern/flags
// Returns the pattern and flags separately, and a boolean indicating success.
// Advances the lexer's position to *after* the closing slash and flags if successful.
// If unsuccessful, the lexer position is restored to before the opening slash.
func (l *Lexer) readRegexLiteral() (pattern string, flags string, success bool, foundComplete bool) {
	var patternBuilder strings.Builder
	var flagsBuilder strings.Builder

	// Save the current position in case we need to backtrack
	savedPosition := l.position
	savedReadPosition := l.readPosition
	savedCh := l.ch
	savedLine := l.line
	savedColumn := l.column

	debugPrintf("readRegexLiteral: starting at position %d, ch='%c'", l.position, l.ch)

	// Consume the opening slash
	l.readChar()

	// Read the pattern
	for {
		// Check for termination conditions
		if l.ch == '/' {
			// Found closing slash - now read flags
			l.readChar() // Consume the closing slash

			// Read flags (letters only)
			for isLetter(l.ch) {
				flagsBuilder.WriteByte(l.ch)
				l.readChar()
			}

			// Validate flags - check for duplicates and invalid flags
			flagsStr := flagsBuilder.String()
			seenFlags := make(map[byte]bool)
			for i := 0; i < len(flagsStr); i++ {
				flag := flagsStr[i]
				// Valid JavaScript regex flags: g, i, m, s, u, y
				if flag != 'g' && flag != 'i' && flag != 'm' && flag != 's' && flag != 'u' && flag != 'y' {
					debugPrintf("readRegexLiteral: invalid flag '%c' found, backtracking", flag)
					// Backtrack on invalid flag
					l.position = savedPosition
					l.readPosition = savedReadPosition
					l.ch = savedCh
					l.line = savedLine
					l.column = savedColumn
					return "", "", false, true // Invalid flag but complete regex
				}
				if seenFlags[flag] {
					// Backtrack on duplicate flag
					l.position = savedPosition
					l.readPosition = savedReadPosition
					l.ch = savedCh
					l.line = savedLine
					l.column = savedColumn
					return "", "", false, true // Duplicate flag but complete regex
				}
				seenFlags[flag] = true
			}

			return patternBuilder.String(), flagsStr, true, true
		}

		if l.ch == 0 { // EOF
			// Backtrack on EOF
			l.position = savedPosition
			l.readPosition = savedReadPosition
			l.ch = savedCh
			l.line = savedLine
			l.column = savedColumn
			return "", "", false, false // Unterminated regex
		}

		if l.ch == '\n' || l.ch == '\r' {
			// Backtrack on newline
			l.position = savedPosition
			l.readPosition = savedReadPosition
			l.ch = savedCh
			l.line = savedLine
			l.column = savedColumn
			return "", "", false, false // Unescaped newline in regex
		}

		if l.ch == '\\' { // Handle escape sequences
			patternBuilder.WriteByte('\\')
			l.readChar() // Consume the backslash

			if l.ch == 0 { // EOF after backslash
				// Backtrack on EOF after backslash
				l.position = savedPosition
				l.readPosition = savedReadPosition
				l.ch = savedCh
				l.line = savedLine
				l.column = savedColumn
				return "", "", false, false // EOF after backslash
			}

			// In regex, we preserve the escape sequence as-is
			// The regex engine will interpret it
			patternBuilder.WriteByte(l.ch)
		} else {
			// Regular character
			patternBuilder.WriteByte(l.ch)
		}

		l.readChar()
	}
}

// skipComment reads until the end of the line.
func (l *Lexer) skipComment() {
	for l.ch != '\n' && l.ch != 0 {
		l.readChar()
	}
	// Don't skip the newline itself, let skipWhitespace handle it
}

// skipHashbangComment reads until the end of the line (similar to skipComment).
// Hashbang comments start with #! and are only valid at the very beginning of a file.
func (l *Lexer) skipHashbangComment() {
	// Consume the opening '#!'
	l.readChar() // Consume '#'
	l.readChar() // Consume '!'

	// Skip until end of line
	for l.ch != '\n' && l.ch != 0 {
		l.readChar()
	}
	// Don't skip the newline itself, let skipWhitespace handle it
}

// skipMultilineComment reads until the end of the multiline comment.
// It consumes the opening '/*' and the closing '*/'.
// Returns true if the comment is terminated successfully, false otherwise (EOF reached).
func (l *Lexer) skipMultilineComment() bool {
	startLine := l.line // For potential error message

	// Consume the opening '/*'
	l.readChar() // Consume '/'
	l.readChar() // Consume '*'

	for {
		if l.ch == 0 { // Reached EOF before finding closing */
			// Error or warning could be logged here about the unterminated comment starting at startLine
			fmt.Printf("Lexer Warning: Unterminated multiline comment starting on line %d\n", startLine)
			return false
		}

		if l.ch == '*' && l.peekChar() == '/' {
			// Found closing */
			l.readChar() // Consume '*'
			l.readChar() // Consume '/'
			return true
		}

		// Consume the current character. readChar() handles line counting.
		l.readChar()
	}
}

// isLetter checks if the character is a letter or underscore.
func isLetter(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_'
}

// isUnicodeIDStart checks if a rune can start a JavaScript identifier
// JavaScript identifiers can start with: letters, $, _, and certain Unicode categories
func isUnicodeIDStart(r rune) bool {
	// ASCII fast path
	if r < 128 {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || r == '$'
	}

	// Standard Unicode Letter categories
	if unicode.IsLetter(r) {
		return true
	}

	// Special cases: $ and _
	if r == '$' || r == '_' {
		return true
	}

	// ECMAScript Other_ID_Start characters
	// These are specific Unicode code points that are allowed to start identifiers
	switch r {
	case 0x2118, // ℘ SCRIPT CAPITAL P
		0x212E, // ℮ ESTIMATED SYMBOL
		0x309B, // ゛ KATAKANA-HIRAGANA VOICED SOUND MARK
		0x309C, // ゜ KATAKANA-HIRAGANA SEMI-VOICED SOUND MARK
		0x1885, // ᢅ (Unicode 9.0)
		0x1886: // ᢆ (Unicode 9.0)
		return true
	}

	return false
}

// isUnicodeIDContinue checks if a rune can continue a JavaScript identifier
// JavaScript identifiers can continue with: ID_Start characters, digits, and certain Unicode categories
func isUnicodeIDContinue(r rune) bool {
	// ASCII fast path
	if r < 128 {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '$'
	}

	// If it can start an identifier, it can also continue one
	if isUnicodeIDStart(r) {
		return true
	}

	// Standard Unicode categories for continuation
	if unicode.IsDigit(r) || unicode.IsMark(r) {
		return true
	}

	// ECMAScript Other_ID_Continue characters
	// These are specific Unicode code points that can continue (but not start) identifiers
	switch r {
	case 0x00B7, // · MIDDLE DOT
		0x0387, // · GREEK ANO TELEIA
		0x1369, 0x136A, 0x136B, 0x136C, 0x136D, 0x136E, 0x136F, 0x1370, 0x1371, // Ethiopian digits
		0x19DA, // ᧚ NEW TAI LUE THAM DIGIT ONE
		0x200C, // ZWNJ - Zero Width Non-Joiner
		0x200D: // ZWJ - Zero Width Joiner
		return true
	}

	return false
}

// canStartIdentifier checks if the current lexer position can start an identifier
// This includes ASCII letters, $, _, \ (for unicode escapes), and Unicode identifier start characters
func (l *Lexer) canStartIdentifier() bool {
	// ASCII fast path
	if l.ch < 128 {
		return isLetter(l.ch) || l.ch == '$' || (l.ch == '\\' && l.peekChar() == 'u')
	}

	// Unicode character - decode and check
	remaining := []byte(l.input[l.position:])
	r, _ := utf8.DecodeRune(remaining)
	if r == utf8.RuneError {
		return false
	}

	return isUnicodeIDStart(r)
}

// canBeRegexStart checks if the previous token allows a regex literal to start
// In JavaScript, regex literals can appear after certain tokens but not others
func canBeRegexStart(prevTokenType TokenType) bool {
	switch prevTokenType {
	// After operators
	case ASSIGN, PLUS_ASSIGN, MINUS_ASSIGN, ASTERISK_ASSIGN, SLASH_ASSIGN,
		REMAINDER_ASSIGN, EXPONENT_ASSIGN, LEFT_SHIFT_ASSIGN, RIGHT_SHIFT_ASSIGN,
		UNSIGNED_RIGHT_SHIFT_ASSIGN, BITWISE_AND_ASSIGN, BITWISE_OR_ASSIGN,
		BITWISE_XOR_ASSIGN, LOGICAL_AND_ASSIGN, LOGICAL_OR_ASSIGN, COALESCE_ASSIGN,
		EQ, NOT_EQ, STRICT_EQ, STRICT_NOT_EQ, LT, GT, LE, GE,
		PLUS, MINUS, ASTERISK, REMAINDER, EXPONENT,
		BITWISE_AND, PIPE, BITWISE_XOR, BITWISE_NOT,
		LEFT_SHIFT, RIGHT_SHIFT, UNSIGNED_RIGHT_SHIFT,
		LOGICAL_AND, LOGICAL_OR, BANG, COALESCE, QUESTION:
		return true
	// After delimiters
	case LPAREN, LBRACKET, LBRACE, COMMA, SEMICOLON, COLON:
		return true
	// After keywords
	case RETURN, THROW, NEW, DELETE, TYPEOF, VOID, IF, ELSE, WHILE, DO, FOR, WITH:
		return true
	// After arrows
	case ARROW:
		return true
	// Special case: beginning of input
	case ILLEGAL:
		return true
	default:
		return false
	}
}

// isDigit checks if the character is a digit.
func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

// isHexDigit checks if the character is a hexadecimal digit (0-9, a-f, A-F).
func isHexDigit(ch byte) bool {
	return ('0' <= ch && ch <= '9') || ('a' <= ch && ch <= 'f') || ('A' <= ch && ch <= 'F')
}

// isOctalDigit checks if the character is an octal digit (0-7).
func isOctalDigit(ch byte) bool {
	return '0' <= ch && ch <= '7'
}

// isBinaryDigit checks if the character is a binary digit (0-1).
func isBinaryDigit(ch byte) bool {
	return ch == '0' || ch == '1'
}

// isDigitForBase checks if the character is a valid digit for the given base.
func isDigitForBase(ch byte, base int) bool {
	switch base {
	case 16:
		return isHexDigit(ch)
	case 10:
		return isDigit(ch)
	case 8:
		return isOctalDigit(ch)
	case 2:
		return isBinaryDigit(ch)
	default:
		return false
	}
}

// readTemplateLiteral handles template literal tokenization
// Returns the appropriate token based on current template state
func (l *Lexer) readTemplateLiteral(startLine, startCol, startPos int) Token {
	debugPrintf("readTemplateLiteral: ENTER - inTemplate=%v, braceDepth=%d, ch='%c'", l.inTemplate, l.braceDepth, l.ch)
	if !l.inTemplate || l.braceDepth > 0 {
		// Opening backtick - start of template literal
		// Push current template state onto stack if we're already in a template
		if l.inTemplate {
			l.templateStack = append(l.templateStack, templateState{
				inTemplate:    l.inTemplate,
				braceDepth:    l.braceDepth,
				templateStart: l.templateStart,
			})
		}

		// Start new template literal
		l.inTemplate = true
		l.templateStart = startPos
		l.braceDepth = 0

		literal := string(l.ch) // The opening backtick
		l.readChar()            // Consume the backtick
		debugPrintf("readTemplateLiteral: OPENING - returning TEMPLATE_START, new ch='%c', position=%d", l.ch, l.position)

		return Token{
			Type:     TEMPLATE_START,
			Literal:  literal,
			Line:     startLine,
			Column:   startCol,
			StartPos: startPos,
			EndPos:   l.position,
		}
	} else {
		// Closing backtick - end of template literal
		debugPrintf("readTemplateLiteral: CLOSING - before readChar, ch='%c', position=%d", l.ch, l.position)
		literal := string(l.ch) // The closing backtick
		l.readChar()            // Consume the backtick
		debugPrintf("readTemplateLiteral: CLOSING - after readChar, ch='%c', position=%d", l.ch, l.position)

		// Pop previous template state from stack if it exists
		if len(l.templateStack) > 0 {
			debugPrintf("readTemplateLiteral: CLOSING - has template stack, length=%d", len(l.templateStack))
			prevState := l.templateStack[len(l.templateStack)-1]
			l.templateStack = l.templateStack[:len(l.templateStack)-1]
			l.inTemplate = prevState.inTemplate
			l.braceDepth = prevState.braceDepth
			l.templateStart = prevState.templateStart
			debugPrintf("readTemplateLiteral: CLOSING - popped state, inTemplate=%v", l.inTemplate)
		} else {
			debugPrintf("readTemplateLiteral: CLOSING - no template stack, resetting state")
			// No previous template state, we're completely done with templates
			l.inTemplate = false
			l.braceDepth = 0
			debugPrintf("readTemplateLiteral: CLOSING - reset state, inTemplate=%v", l.inTemplate)
		}

		debugPrintf("readTemplateLiteral: CLOSING - creating TEMPLATE_END token")
		result := Token{
			Type:     TEMPLATE_END,
			Literal:  literal,
			Line:     startLine,
			Column:   startCol,
			StartPos: startPos,
			EndPos:   l.position,
		}
		debugPrintf("readTemplateLiteral: CLOSING - returning TEMPLATE_END token")
		return result
	}
}

// readTemplateString reads string content within a template literal
// Stops at: backtick (`), interpolation start (${), or EOF
func (l *Lexer) readTemplateString(startLine, startCol, startPos int) Token {
	var builder strings.Builder

	for {
		// Stop conditions
		if l.ch == 0 { // EOF
			// Unterminated template literal - advance to prevent infinite loop
			l.readChar() // Advance past EOF to prevent getting stuck
			return Token{
				Type:     ILLEGAL,
				Literal:  "Unterminated template literal",
				Line:     startLine,
				Column:   startCol,
				StartPos: startPos,
				EndPos:   l.position,
			}
		}

		if l.ch == '`' {
			// End of template - don't consume the backtick, let NextToken handle it
			break
		}

		if l.ch == '$' && l.peekChar() == '{' {
			// Start of interpolation - don't consume, let NextToken handle it
			break
		}

		// Handle escape sequences in template strings
		if l.ch == '\\' {
			l.readChar() // Consume backslash
			switch l.ch {
			case 'n':
				builder.WriteByte('\n')
			case 't':
				builder.WriteByte('\t')
			case 'r':
				builder.WriteByte('\r')
			case '\\':
				builder.WriteByte('\\')
			case '`':
				builder.WriteByte('`') // Escaped backtick
			case '$':
				builder.WriteByte('$') // Escaped dollar sign
			case 0: // EOF after backslash
				return Token{
					Type:     ILLEGAL,
					Literal:  "Invalid escape sequence in template literal",
					Line:     startLine,
					Column:   startCol,
					StartPos: startPos,
					EndPos:   l.position,
				}
			default:
				// For other characters, include the backslash (JS behavior)
				builder.WriteByte('\\')
				builder.WriteByte(l.ch)
			}
		} else {
			// Regular character (including newlines, which are allowed in templates)
			builder.WriteByte(l.ch)
		}

		l.readChar()
	}

	// Return the template string token
	return Token{
		Type:     TEMPLATE_STRING,
		Literal:  builder.String(),
		Line:     startLine,
		Column:   startCol,
		StartPos: startPos,
		EndPos:   l.position,
	}
}

// GetSource returns the source file associated with this lexer
func (l *Lexer) GetSource() *source.SourceFile {
	return l.source
}

// --- TODO: Implement readString for string literals --- // Removed TODO
