package lexer

import (
	"fmt"
	"strings" // Added for strings.Builder
)

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
	IDENT     TokenType = "IDENT"     // functionName, variableName
	NUMBER    TokenType = "NUMBER"    // 123, 45.67
	STRING    TokenType = "STRING"    // "hello world"
	NULL      TokenType = "NULL"      // Added
	UNDEFINED TokenType = "UNDEFINED" // Added

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

	// Increment/Decrement
	INC TokenType = "++" // Added
	DEC TokenType = "--" // Added

	// Type Operator
	PIPE TokenType = "|" // Added for Union Types

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
	TRUE     TokenType = "TRUE"
	FALSE    TokenType = "FALSE"
	IF       TokenType = "IF"
	ELSE     TokenType = "ELSE"
	RETURN   TokenType = "RETURN"
	WHILE    TokenType = "WHILE"
	DO       TokenType = "DO" // Added for do...while
	FOR      TokenType = "FOR"
	BREAK    TokenType = "BREAK"    // Added
	CONTINUE TokenType = "CONTINUE" // Added
	TYPE     TokenType = "TYPE"     // Added for type aliases
	SWITCH   TokenType = "SWITCH"   // Added for switch statements
	CASE     TokenType = "CASE"     // Added for switch statements
	DEFAULT  TokenType = "DEFAULT"  // Added for switch statements

	// Logical Operators
	LOGICAL_AND TokenType = "&&" // Added
	LOGICAL_OR  TokenType = "||" // Added
	COALESCE    TokenType = "??" // Added

	// New Strict Equality Operators
	STRICT_EQ     TokenType = "==="
	STRICT_NOT_EQ TokenType = "!=="

	// New Ternary Operator Tokens
	QUESTION TokenType = "?"
)

var keywords = map[string]TokenType{
	"function":  FUNCTION,
	"let":       LET,
	"const":     CONST,
	"true":      TRUE,
	"false":     FALSE,
	"if":        IF,
	"else":      ELSE,
	"return":    RETURN,
	"null":      NULL,
	"undefined": UNDEFINED, // Added
	"while":     WHILE,
	"do":        DO, // Added for do...while
	"for":       FOR,
	"break":     BREAK,    // Added
	"continue":  CONTINUE, // Added
	"type":      TYPE,     // Added
	"switch":    SWITCH,   // Added
	"case":      CASE,     // Added
	"default":   DEFAULT,  // Added
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
	input        string
	position     int  // current position in input (points to current char's byte offset)
	readPosition int  // current reading position in input (byte offset after current char)
	ch           byte // current char under examination
	line         int  // current 1-based line number
	column       int  // current 1-based column number (position of l.position on l.line)
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
}

// NewLexer creates a new Lexer.
func NewLexer(input string) *Lexer {
	l := &Lexer{input: input, line: 1, column: 1} // Start at line 1, column 1
	l.readChar()                                  // Initialize l.ch, l.position, l.readPosition, and potentially update line/column if input starts with newline
	return l
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

// skipWhitespace consumes whitespace characters (space, tab, newline, carriage return).
// It relies on readChar to update line and column counts.
func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		// The line/column update happens inside readChar
		l.readChar()
	}
}

// NextToken scans the input and returns the next token.
func (l *Lexer) NextToken() Token {
	var tok Token

	l.skipWhitespace()

	// Capture token start position *after* skipping whitespace
	startLine := l.line
	startCol := l.column
	startPos := l.position

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
		if l.peekChar() == '=' { // Check for *=
			l.readChar()                                // Consume '='
			literal := l.input[startPos : l.position+1] // Read the actual '*='
			l.readChar()                                // Advance past '='
			tok = Token{Type: ASTERISK_ASSIGN, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else {
			literal := string(l.ch) // Just '*'
			l.readChar()            // Advance past '*'
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
		} else {
			literal := string(l.ch) // Just '/'
			l.readChar()            // Advance past '/'
			tok = Token{Type: SLASH, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '&':
		if l.peekChar() == '&' { // Check for &&
			l.readChar()                                // Consume second '&'
			literal := l.input[startPos : l.position+1] // Read the actual '&&'
			l.readChar()                                // Advance past '&'
			tok = Token{Type: LOGICAL_AND, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else {
			// Single '&' is illegal for now
			literal := string(l.ch)
			l.readChar()
			tok = Token{Type: ILLEGAL, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '|':
		if l.peekChar() == '|' { // Check for ||
			l.readChar()                                // Consume second '|'
			literal := l.input[startPos : l.position+1] // Read the actual '||'
			l.readChar()                                // Advance past '|'
			tok = Token{Type: LOGICAL_OR, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else {
			// Single '|' is Union Type
			literal := string(l.ch)
			l.readChar()
			tok = Token{Type: PIPE, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '<':
		if l.peekChar() == '=' {
			l.readChar()                                // Consume '='
			literal := l.input[startPos : l.position+1] // Read the actual '<='
			l.readChar()                                // Advance past '='
			tok = Token{Type: LE, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else {
			literal := string(l.ch) // Just '<'
			l.readChar()            // Advance past '<'
			tok = Token{Type: LT, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	case '>':
		if l.peekChar() == '=' {
			l.readChar()                                // Consume '='
			literal := l.input[startPos : l.position+1] // Read the actual '>='
			l.readChar()                                // Advance past '='
			tok = Token{Type: GE, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else {
			literal := string(l.ch) // Just '>'
			l.readChar()            // Advance past '>'
			tok = Token{Type: GT, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
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
		if l.peekChar() == '?' { // Check for ??
			l.readChar()                                // Consume second '?'
			literal := l.input[startPos : l.position+1] // Read the actual '??'
			l.readChar()                                // Advance past '?'
			tok = Token{Type: COALESCE, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		} else {
			// Original ternary operator
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
	case 0: // EOF
		tok = Token{Type: EOF, Literal: "", Line: startLine, Column: startCol, StartPos: startPos, EndPos: startPos}
	default:
		if isLetter(l.ch) {
			literal := l.readIdentifier() // Consumes letters/digits/_
			tokType := LookupIdent(literal)
			// readIdentifier leaves l.position *after* the last char of the identifier
			tok = Token{Type: tokType, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			return tok // Return early, readIdentifier already called readChar()
		} else if isDigit(l.ch) {
			literal := l.readNumber() // Consumes digits and potentially '.'
			// readNumber leaves l.position *after* the last char of the number
			tok = Token{Type: NUMBER, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
			return tok // Return early, readNumber already called readChar()
		} else {
			// Illegal character
			literal := string(l.ch)
			l.readChar() // Consume the illegal character
			tok = Token{Type: ILLEGAL, Literal: literal, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
		}
	}

	return tok
}

// readIdentifier reads an identifier (letters, digits, _) and advances the lexer's position.
// It returns the literal string found.
func (l *Lexer) readIdentifier() string {
	startPos := l.position
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}
	return l.input[startPos:l.position]
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
			case '\\':
				builder.WriteByte('\\')
			case quote: // Handle escaped quote (' or ")
				builder.WriteByte(quote)
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

// skipComment reads until the end of the line.
func (l *Lexer) skipComment() {
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

		// Handle newlines inside the comment
		if l.ch == '\n' {
			l.line++
		}

		// Consume the current character
		l.readChar()
	}
}

// isLetter checks if the character is a letter or underscore.
func isLetter(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_'
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

// --- TODO: Implement readString for string literals --- // Removed TODO
