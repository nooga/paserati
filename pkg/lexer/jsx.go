package lexer

import (
	"unicode/utf8"
)

// JSX_TEXT is the text between JSX tags and expression containers.
const JSX_TEXT TokenType = "JSX_TEXT"

// JSX is scanned on the parser's demand, from where the lexer stands: inside a
// tag (names, attributes, punctuation) and between tags (text). The parser
// rewinds the lexer to the end of the token it has consumed before switching
// to one of these, since its lookahead was scanned as ordinary code.

// ScanJSXTagToken scans the next token inside a JSX tag: a name (an
// identifier that may contain '-'), a string (no escapes, as in HTML), or
// punctuation. '{' goes through NextToken so template literal brace tracking
// stays balanced with the '}' the expression parser consumes.
func (l *Lexer) ScanJSXTagToken() Token {
	if l.pushedToken != nil {
		return l.NextToken()
	}
	l.skipJSXTrivia()
	startLine, startCol, startPos := l.line, l.column, l.position
	single := func(t TokenType) Token {
		lit := l.input[startPos : startPos+1]
		l.readChar()
		l.prevToken = t
		return Token{Type: t, Literal: lit, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
	}
	if l.isEOF() {
		return Token{Type: EOF, Line: startLine, Column: startCol, StartPos: startPos, EndPos: startPos}
	}
	switch l.ch {
	case '>':
		return single(GT)
	case '/':
		return single(SLASH)
	case '=':
		return single(ASSIGN)
	case '.':
		return single(DOT)
	case ':':
		return single(COLON)
	case '<':
		return single(LT)
	case '{':
		return l.NextToken()
	case '"', '\'':
		quote := l.ch
		l.readChar()
		valStart := l.position
		for !l.isEOF() && l.ch != quote {
			l.readChar()
		}
		val := l.input[valStart:l.position]
		tok := Token{Type: STRING, Literal: val, Line: startLine, Column: startCol, StartPos: startPos}
		if l.isEOF() {
			tok.Diags = []TokenDiag{{Line: startLine, Column: startCol, StartPos: startPos, Code: "TS1002", Msg: "Unterminated string literal."}}
		} else {
			l.readChar()
		}
		tok.EndPos = l.position
		l.prevToken = STRING
		return tok
	}
	if l.atJSXIdentifierStart() {
		for !l.isEOF() && (l.ch == '-' || l.atJSXIdentifierPart()) {
			if l.ch >= utf8.RuneSelf {
				_, size := utf8.DecodeRuneInString(l.input[l.position:])
				for i := 0; i < size; i++ {
					l.readChar()
				}
				continue
			}
			l.readChar()
		}
		lit := l.input[startPos:l.position]
		l.prevToken = IDENT
		return Token{Type: IDENT, Literal: lit, Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
	}
	return l.NextToken()
}

// ScanJSXChild scans the next child of a JSX element: '<' (a tag), '{' (an
// expression container) or the text up to either.
func (l *Lexer) ScanJSXChild() Token {
	if l.pushedToken != nil {
		return l.NextToken()
	}
	startLine, startCol, startPos := l.line, l.column, l.position
	if l.isEOF() {
		return Token{Type: EOF, Line: startLine, Column: startCol, StartPos: startPos, EndPos: startPos}
	}
	switch l.ch {
	case '<':
		l.readChar()
		l.prevToken = LT
		return Token{Type: LT, Literal: "<", Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
	case '{':
		return l.NextToken()
	}
	for !l.isEOF() && l.ch != '<' && l.ch != '{' {
		l.readChar()
	}
	l.prevToken = JSX_TEXT
	return Token{Type: JSX_TEXT, Literal: l.input[startPos:l.position], Line: startLine, Column: startCol, StartPos: startPos, EndPos: l.position}
}

// skipJSXTrivia skips whitespace and comments inside a tag.
func (l *Lexer) skipJSXTrivia() {
	for {
		l.skipWhitespace()
		if l.ch == '/' && l.peekChar() == '/' {
			l.skipComment()
			continue
		}
		if l.ch == '/' && l.peekChar() == '*' {
			if !l.skipMultilineComment() {
				return
			}
			continue
		}
		return
	}
}

func (l *Lexer) atJSXIdentifierStart() bool {
	if l.ch < utf8.RuneSelf {
		return isLetter(l.ch) || l.ch == '$'
	}
	r, _ := utf8.DecodeRuneInString(l.input[l.position:])
	return isUnicodeIDStart(r)
}

func (l *Lexer) atJSXIdentifierPart() bool {
	if l.ch < utf8.RuneSelf {
		return isLetter(l.ch) || l.ch == '$' || isDigit(l.ch)
	}
	r, _ := utf8.DecodeRuneInString(l.input[l.position:])
	return isUnicodeIDContinue(r)
}

// Seek moves the lexer to byte offset pos, which is on line and column, keeping
// the template literal state. The parser uses it to rescan from the end of a
// token it consumed whose line and column it knows.
func (l *Lexer) Seek(pos, line, column int) {
	l.pushedToken = nil
	l.line, l.column = line, column
	if pos >= len(l.input) {
		l.position, l.readPosition, l.ch = len(l.input), len(l.input)+1, 0
		return
	}
	l.position, l.readPosition, l.ch = pos, pos+1, l.input[pos]
}
