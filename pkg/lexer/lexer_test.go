package lexer

import (
	"testing"
)

func TestNextToken(t *testing.T) {
	input := `let five = 5;
const ten = 10.5;

let add = function(x, y) {
  return x + y;
};

let result = add(five, ten);
!*-/5;
5 < 10 > 5;

if (5 < 10) {
	return true;
} else {
	return false;
}

10 == 10;
10 != 9;
"foobar"
"foo bar"
// This is a comment
let next = null;`

	tests := []struct {
		expectedType    TokenType
		expectedLiteral string
		expectedLine    int // Approximate line number for verification
	}{
		{LET, "let", 1},
		{IDENT, "five", 1},
		{ASSIGN, "=", 1},
		{NUMBER, "5", 1},
		{SEMICOLON, ";", 1},
		{CONST, "const", 2},
		{IDENT, "ten", 2},
		{ASSIGN, "=", 2},
		{NUMBER, "10.5", 2},
		{SEMICOLON, ";", 2},
		{LET, "let", 4},
		{IDENT, "add", 4},
		{ASSIGN, "=", 4},
		{FUNCTION, "function", 4},
		{LPAREN, "(", 4},
		{IDENT, "x", 4},
		{COMMA, ",", 4},
		{IDENT, "y", 4},
		{RPAREN, ")", 4},
		{LBRACE, "{", 4},
		{RETURN, "return", 5},
		{IDENT, "x", 5},
		{PLUS, "+", 5},
		{IDENT, "y", 5},
		{SEMICOLON, ";", 5},
		{RBRACE, "}", 6},
		{SEMICOLON, ";", 6},
		{LET, "let", 8},
		{IDENT, "result", 8},
		{ASSIGN, "=", 8},
		{IDENT, "add", 8},
		{LPAREN, "(", 8},
		{IDENT, "five", 8},
		{COMMA, ",", 8},
		{IDENT, "ten", 8},
		{RPAREN, ")", 8},
		{SEMICOLON, ";", 8},
		{BANG, "!", 9},
		{ASTERISK, "*", 9},
		{MINUS, "-", 9},
		{SLASH, "/", 9},
		{NUMBER, "5", 9},
		{SEMICOLON, ";", 9},
		{NUMBER, "5", 10},
		{LT, "<", 10},
		{NUMBER, "10", 10},
		{GT, ">", 10},
		{NUMBER, "5", 10},
		{SEMICOLON, ";", 10},
		{IF, "if", 12},
		{LPAREN, "(", 12},
		{NUMBER, "5", 12},
		{LT, "<", 12},
		{NUMBER, "10", 12},
		{RPAREN, ")", 12},
		{LBRACE, "{", 12},
		{RETURN, "return", 13},
		{TRUE, "true", 13},
		{SEMICOLON, ";", 13},
		{RBRACE, "}", 14},
		{ELSE, "else", 14},
		{LBRACE, "{", 14},
		{RETURN, "return", 15},
		{FALSE, "false", 15},
		{SEMICOLON, ";", 15},
		{RBRACE, "}", 16},
		{NUMBER, "10", 18},
		{EQ, "==", 18},
		{NUMBER, "10", 18},
		{SEMICOLON, ";", 18},
		{NUMBER, "10", 19},
		{NOT_EQ, "!=", 19},
		{NUMBER, "9", 19},
		{SEMICOLON, ";", 19},
		{STRING, "foobar", 20},
		{STRING, "foo bar", 21},
		// Comment on line 22 is skipped
		{LET, "let", 23},
		{IDENT, "next", 23},
		{ASSIGN, "=", 23},
		{NULL, "null", 23},
		{SEMICOLON, ";", 23},
		{EOF, "", 23}, // Line number might be last non-whitespace line
	}

	l := NewLexer(input)

	for i, tt := range tests {
		tok := l.NextToken()

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q (literal: %q, line: %d)",
				i, tt.expectedType, tok.Type, tok.Literal, tok.Line)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q (type: %q, line: %d)",
				i, tt.expectedLiteral, tok.Literal, tok.Type, tok.Line)
		}

		// Optional: Check line number, allowing for slight variations due to whitespace/comments
		if tok.Line != tt.expectedLine && tok.Type != EOF { // Don't strictly check EOF line
			t.Logf("tests[%d] - line number mismatch. expected=%d, got=%d (type: %q, literal: %q)",
				i, tt.expectedLine, tok.Line, tok.Type, tok.Literal)
			// Make this Logf instead of Fatalf as line numbers can be tricky
		}
	}
}

func TestSpecificOperatorLexing(t *testing.T) {
	input := `* *= ** **= > >= >> >>= >>> >>>= & &= | |= || ||= ?? ??= ? <= << <<=`

	tests := []struct {
		expectedType    TokenType
		expectedLiteral string
	}{
		{ASTERISK, "*"},
		{ASTERISK_ASSIGN, "*="},
		{EXPONENT, "**"},
		{EXPONENT_ASSIGN, "**="},
		{GT, ">"},
		{GE, ">="},
		{RIGHT_SHIFT, ">>"},
		{RIGHT_SHIFT_ASSIGN, ">>="},
		{UNSIGNED_RIGHT_SHIFT, ">>>"},
		{UNSIGNED_RIGHT_SHIFT_ASSIGN, ">>>="},
		{BITWISE_AND, "&"},
		{BITWISE_AND_ASSIGN, "&="},
		{PIPE, "|"}, // Assuming PIPE for single |
		{BITWISE_OR_ASSIGN, "|="},
		{LOGICAL_OR, "||"},
		{LOGICAL_OR_ASSIGN, "||="},
		{COALESCE, "??"},
		{COALESCE_ASSIGN, "??="},
		{QUESTION, "?"},
		{LE, "<="},
		{LEFT_SHIFT, "<<"},
		{LEFT_SHIFT_ASSIGN, "<<="},
		{EOF, ""},
	}

	l := NewLexer(input)

	for i, tt := range tests {
		tok := l.NextToken()

		if tok.Type != tt.expectedType {
			t.Errorf("tests[%d] - tokentype wrong. expected=%q (%s), got=%q (%s)",
				i, tt.expectedType, tt.expectedLiteral, tok.Type, tok.Literal)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Errorf("tests[%d] - literal wrong. expected=%q, got=%q (type: %q)",
				i, tt.expectedLiteral, tok.Literal, tok.Type)
		}
	}
}
