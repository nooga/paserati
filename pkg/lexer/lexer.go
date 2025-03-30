package lexer

import (
	"fmt"
)

// TokenType represents the type of a token.
type TokenType string

// Token represents a lexical token.
type Token struct {
	Type    TokenType
	Literal string // The actual text of the token (lexeme)
	Line    int    // Line number where the token starts
}

// --- Token Types ---
const (
	// Special
	ILLEGAL TokenType = "ILLEGAL" // Unknown token/character
	EOF     TokenType = "EOF"     // End Of File

	// Identifiers + Literals
	IDENT  TokenType = "IDENT"  // functionName, variableName
	NUMBER TokenType = "NUMBER" // 123, 45.67
	STRING TokenType = "STRING" // "hello world"

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

	// Delimiters
	COMMA     TokenType = ","
	SEMICOLON TokenType = ";"
	COLON     TokenType = ":"
	LPAREN    TokenType = "("
	RPAREN    TokenType = ")"
	LBRACE    TokenType = "{"
	RBRACE    TokenType = "}"
	ARROW     TokenType = "=>" // Added for arrow functions
	// Add LBRACKET, RBRACKET later for arrays/objects

	// Keywords
	FUNCTION TokenType = "FUNCTION"
	LET      TokenType = "LET"
	CONST    TokenType = "CONST"
	TRUE     TokenType = "TRUE"
	FALSE    TokenType = "FALSE"
	IF       TokenType = "IF"
	ELSE     TokenType = "ELSE"
	RETURN   TokenType = "RETURN"
	NULL     TokenType = "NULL" // Explicit null
)

var keywords = map[string]TokenType{
	"fn":       FUNCTION, // Using 'fn' for brevity like Rust/others?
	"function": FUNCTION,
	"let":      LET,
	"const":    CONST,
	"true":     TRUE,
	"false":    FALSE,
	"if":       IF,
	"else":     ELSE,
	"return":   RETURN,
	"null":     NULL,
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
	position     int  // current position in input (points to current char)
	readPosition int  // current reading position in input (after current char)
	ch           byte // current char under examination
	line         int  // current line number
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
	l := &Lexer{input: input, line: 1}
	l.readChar() // Initialize l.ch, l.position, l.readPosition
	return l
}

// readChar gives us the next character and advances our position in the input string.
func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0 // 0 is ASCII for NUL, signifies EOF
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
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
func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		if l.ch == '\n' {
			l.line++
		}
		l.readChar()
	}
}

// NextToken scans the input and returns the next token.
func (l *Lexer) NextToken() Token {
	var tok Token

	l.skipWhitespace()

	tok.Line = l.line // Assign line number before reading the char for the token

	switch l.ch {
	case '=':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			literal := string(ch) + string(l.ch)
			tok = Token{Type: EQ, Literal: literal, Line: l.line}
		} else if l.peekChar() == '>' {
			ch := l.ch
			l.readChar()
			literal := string(ch) + string(l.ch)
			tok = Token{Type: ARROW, Literal: literal, Line: l.line}
		} else {
			tok = newToken(ASSIGN, l.ch, l.line)
		}
	case '!':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			literal := string(ch) + string(l.ch)
			tok = Token{Type: NOT_EQ, Literal: literal, Line: l.line}
		} else {
			tok = newToken(BANG, l.ch, l.line)
		}
	case '+':
		tok = newToken(PLUS, l.ch, l.line)
	case '-':
		tok = newToken(MINUS, l.ch, l.line)
	case '*':
		tok = newToken(ASTERISK, l.ch, l.line)
	case '/':
		// Handle comments
		if l.peekChar() == '/' {
			l.skipComment()
			return l.NextToken() // Recursively call NextToken after skipping comment
		} else {
			tok = newToken(SLASH, l.ch, l.line)
		}
	case '<':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			literal := string(ch) + string(l.ch)
			tok = Token{Type: LE, Literal: literal, Line: l.line}
		} else {
			tok = newToken(LT, l.ch, l.line)
		}
	case '>':
		// TODO: Add >= later if needed
		tok = newToken(GT, l.ch, l.line)
	case ';':
		tok = newToken(SEMICOLON, l.ch, l.line)
	case ':':
		tok = newToken(COLON, l.ch, l.line)
	case ',':
		tok = newToken(COMMA, l.ch, l.line)
	case '(':
		tok = newToken(LPAREN, l.ch, l.line)
	case ')':
		tok = newToken(RPAREN, l.ch, l.line)
	case '{':
		tok = newToken(LBRACE, l.ch, l.line)
	case '}':
		tok = newToken(RBRACE, l.ch, l.line)
	case '"':
		tok.Type = STRING
		tok.Literal = l.readString()
		// readString advances the lexer past the closing quote
		return tok // Early return
	case 0: // EOF
		tok.Literal = ""
		tok.Type = EOF
	default:
		if isLetter(l.ch) {
			tok.Literal = l.readIdentifier()
			tok.Type = LookupIdent(tok.Literal)
			return tok // Early return for identifiers/keywords
		} else if isDigit(l.ch) {
			tok.Type = NUMBER
			tok.Literal = l.readNumber()
			return tok // Early return for numbers
		} else {
			tok = newToken(ILLEGAL, l.ch, l.line)
		}
	}

	l.readChar() // Advance the lexer for the next token
	return tok
}

// newToken is a helper to create a Token for a single character.
func newToken(tokenType TokenType, ch byte, line int) Token {
	return Token{Type: tokenType, Literal: string(ch), Line: line}
}

// readIdentifier reads an identifier (letters, digits, _) and advances the lexer's position.
func (l *Lexer) readIdentifier() string {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.input[position:l.position]
}

// readNumber reads a number (integer or float) and advances the lexer's position.
func (l *Lexer) readNumber() string {
	position := l.position
	// Read integer part
	for isDigit(l.ch) {
		l.readChar()
	}

	// Look for a fractional part
	if l.ch == '.' && isDigit(l.peekChar()) {
		// Consume the "."
		l.readChar()

		// Read fractional part
		for isDigit(l.ch) {
			l.readChar()
		}
	}
	// TODO: Add exponent support (e.g., 1.23e-4)
	return l.input[position:l.position]
}

// readString reads a double-quoted string literal.
// It consumes characters until the closing quote or EOF.
// Does not currently handle escape sequences.
func (l *Lexer) readString() string {
	// Store starting line for potential error reporting
	startLine := l.line
	// Current position is the opening quote, advance past it.
	position := l.position + 1
	for {
		l.readChar()
		if l.ch == '"' || l.ch == 0 { // Found closing quote or EOF
			break
		}
		// Handle newlines within strings (update line count)
		if l.ch == '\n' {
			l.line++
		}
	}

	if l.ch == 0 { // Check for unterminated string
		// TODO: Better error handling? Return ILLEGAL token?
		// For now, return the partial string read.
		// The parser would likely catch this as an error later.
		fmt.Printf("Lexer Warning: Unterminated string starting on line %d\n", startLine)
		return l.input[position:l.position] // Return what we got before EOF
	}

	strContent := l.input[position:l.position]
	l.readChar() // Consume the closing quote for the *next* call to NextToken
	return strContent
}

// skipComment reads until the end of the line.
func (l *Lexer) skipComment() {
	for l.ch != '\n' && l.ch != 0 {
		l.readChar()
	}
	// Don't skip the newline itself, let skipWhitespace handle it
}

// isLetter checks if the character is a letter or underscore.
func isLetter(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_'
}

// isDigit checks if the character is a digit.
func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

// --- TODO: Implement readString for string literals ---
