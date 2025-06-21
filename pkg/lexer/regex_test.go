package lexer

import (
	"testing"
)

func TestRegexLiterals(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []TokenType
		literals []string
	}{
		{
			name:     "Simple regex",
			input:    "/hello/",
			expected: []TokenType{REGEX_LITERAL, EOF},
			literals: []string{"/hello/", ""},
		},
		{
			name:     "Regex with flags",
			input:    "/world/gi",
			expected: []TokenType{REGEX_LITERAL, EOF},
			literals: []string{"/world/gi", ""},
		},
		{
			name:     "Complex regex",
			input:    "/complex[A-Z]+/m",
			expected: []TokenType{REGEX_LITERAL, EOF},
			literals: []string{"/complex[A-Z]+/m", ""},
		},
		{
			name:     "Assignment context",
			input:    "let x = /test/i;",
			expected: []TokenType{LET, IDENT, ASSIGN, REGEX_LITERAL, SEMICOLON, EOF},
			literals: []string{"let", "x", "=", "/test/i", ";", ""},
		},
		{
			name:     "Division vs regex - division",
			input:    "5 / 2",
			expected: []TokenType{NUMBER, SLASH, NUMBER, EOF},
			literals: []string{"5", "/", "2", ""},
		},
		{
			name:     "Division vs regex - regex after paren",
			input:    "(/pattern/)",
			expected: []TokenType{LPAREN, REGEX_LITERAL, RPAREN, EOF},
			literals: []string{"(", "/pattern/", ")", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewLexer(tt.input)
			
			for i, expectedToken := range tt.expected {
				tok := l.NextToken()
				if tok.Type != expectedToken {
					t.Errorf("test[%d] - tokentype wrong. expected=%q, got=%q", i, expectedToken, tok.Type)
				}
				if i < len(tt.literals) && tok.Literal != tt.literals[i] {
					t.Errorf("test[%d] - literal wrong. expected=%q, got=%q", i, tt.literals[i], tok.Literal)
				}
			}
		})
	}
}

func TestRegexFlags(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Valid flags",
			input:   "/test/gims",
			wantErr: false,
		},
		{
			name:    "Invalid flag",
			input:   "/test/x",
			wantErr: true,
		},
		{
			name:    "Duplicate flag",
			input:   "/test/gg",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewLexer(tt.input)
			tok := l.NextToken()
			
			if tt.wantErr {
				if tok.Type != ILLEGAL {
					t.Errorf("expected ILLEGAL token for invalid regex, got %q", tok.Type)
				}
			} else {
				if tok.Type != REGEX_LITERAL {
					t.Errorf("expected REGEX_LITERAL token, got %q", tok.Type)
				}
			}
		})
	}
}