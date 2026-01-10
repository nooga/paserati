package tests

import (
	"testing"

	"github.com/nooga/paserati/pkg/lexer"
)

const templateDebug = false // Enable for debugging

// templateTokenTestCase represents a single template literal tokenization test
type templateTokenTestCase struct {
	name     string
	input    string
	expected []tokenExpectation
}

// tokenExpectation represents an expected token
type tokenExpectation struct {
	tokenType lexer.TokenType
	literal   string
}

func TestTemplateLiteralTokenization(t *testing.T) {
	testCases := []templateTokenTestCase{
		{
			name:  "Simple Template",
			input: "`hello world`",
			expected: []tokenExpectation{
				{lexer.TEMPLATE_START, "`"},
				{lexer.TEMPLATE_STRING, "hello world"},
				{lexer.TEMPLATE_END, "`"},
				{lexer.EOF, ""},
			},
		},
		{
			name:  "Empty Template",
			input: "``",
			expected: []tokenExpectation{
				{lexer.TEMPLATE_START, "`"},
				{lexer.TEMPLATE_END, "`"},
				{lexer.EOF, ""},
			},
		},
		{
			name:  "Simple Interpolation",
			input: "`hello ${name}`",
			expected: []tokenExpectation{
				{lexer.TEMPLATE_START, "`"},
				{lexer.TEMPLATE_STRING, "hello "},
				{lexer.TEMPLATE_INTERPOLATION, "${"},
				{lexer.IDENT, "name"},
				{lexer.RBRACE, "}"},
				{lexer.TEMPLATE_END, "`"},
				{lexer.EOF, ""},
			},
		},
		{
			name:  "Multiple Interpolations",
			input: "`${x} + ${y} = ${result}`",
			expected: []tokenExpectation{
				{lexer.TEMPLATE_START, "`"},
				{lexer.TEMPLATE_INTERPOLATION, "${"},
				{lexer.IDENT, "x"},
				{lexer.RBRACE, "}"},
				{lexer.TEMPLATE_STRING, " + "},
				{lexer.TEMPLATE_INTERPOLATION, "${"},
				{lexer.IDENT, "y"},
				{lexer.RBRACE, "}"},
				{lexer.TEMPLATE_STRING, " = "},
				{lexer.TEMPLATE_INTERPOLATION, "${"},
				{lexer.IDENT, "result"},
				{lexer.RBRACE, "}"},
				{lexer.TEMPLATE_END, "`"},
				{lexer.EOF, ""},
			},
		},
		{
			name:  "Complex Expression Interpolation",
			input: "`Result: ${x + y * 2}`",
			expected: []tokenExpectation{
				{lexer.TEMPLATE_START, "`"},
				{lexer.TEMPLATE_STRING, "Result: "},
				{lexer.TEMPLATE_INTERPOLATION, "${"},
				{lexer.IDENT, "x"},
				{lexer.PLUS, "+"},
				{lexer.IDENT, "y"},
				{lexer.ASTERISK, "*"},
				{lexer.NUMBER, "2"},
				{lexer.RBRACE, "}"},
				{lexer.TEMPLATE_END, "`"},
				{lexer.EOF, ""},
			},
		},
		{
			name:  "Nested Braces in Interpolation",
			input: "`Value: ${obj.method({x: 1})}`",
			expected: []tokenExpectation{
				{lexer.TEMPLATE_START, "`"},
				{lexer.TEMPLATE_STRING, "Value: "},
				{lexer.TEMPLATE_INTERPOLATION, "${"},
				{lexer.IDENT, "obj"},
				{lexer.DOT, "."},
				{lexer.IDENT, "method"},
				{lexer.LPAREN, "("},
				{lexer.LBRACE, "{"},
				{lexer.IDENT, "x"},
				{lexer.COLON, ":"},
				{lexer.NUMBER, "1"},
				{lexer.RBRACE, "}"},
				{lexer.RPAREN, ")"},
				{lexer.RBRACE, "}"},
				{lexer.TEMPLATE_END, "`"},
				{lexer.EOF, ""},
			},
		},
		{
			name:  "Escaped Characters",
			input: "`hello \\` world \\${not interpolation}`",
			expected: []tokenExpectation{
				{lexer.TEMPLATE_START, "`"},
				{lexer.TEMPLATE_STRING, "hello ` world ${not interpolation}"},
				{lexer.TEMPLATE_END, "`"},
				{lexer.EOF, ""},
			},
		},
		{
			name:  "Multiline Template",
			input: "`line 1\nline 2\nline 3`",
			expected: []tokenExpectation{
				{lexer.TEMPLATE_START, "`"},
				{lexer.TEMPLATE_STRING, "line 1\nline 2\nline 3"},
				{lexer.TEMPLATE_END, "`"},
				{lexer.EOF, ""},
			},
		},
		{
			name:  "Template with Escape Sequences",
			input: "`tab\\there\\nnewline`",
			expected: []tokenExpectation{
				{lexer.TEMPLATE_START, "`"},
				{lexer.TEMPLATE_STRING, "tab\there\nnewline"},
				{lexer.TEMPLATE_END, "`"},
				{lexer.EOF, ""},
			},
		},
		{
			name:  "Only Interpolations",
			input: "`${a}${b}${c}`",
			expected: []tokenExpectation{
				{lexer.TEMPLATE_START, "`"},
				{lexer.TEMPLATE_INTERPOLATION, "${"},
				{lexer.IDENT, "a"},
				{lexer.RBRACE, "}"},
				{lexer.TEMPLATE_INTERPOLATION, "${"},
				{lexer.IDENT, "b"},
				{lexer.RBRACE, "}"},
				{lexer.TEMPLATE_INTERPOLATION, "${"},
				{lexer.IDENT, "c"},
				{lexer.RBRACE, "}"},
				{lexer.TEMPLATE_END, "`"},
				{lexer.EOF, ""},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if templateDebug {
				t.Logf("Testing: %s", tc.input)
			}

			l := lexer.NewLexer(tc.input)

			for i, expected := range tc.expected {
				tok := l.NextToken()

				if tok.Type != expected.tokenType {
					t.Errorf("Token %d: expected type %s, got %s", i, expected.tokenType, tok.Type)
				}

				if tok.Literal != expected.literal {
					t.Errorf("Token %d: expected literal %q, got %q", i, expected.literal, tok.Literal)
				}

				if templateDebug {
					t.Logf("Token %d: %s: %q", i, tok.Type, tok.Literal)
				}
			}
		})
	}
}

func TestTemplateLiteralErrors(t *testing.T) {
	errorCases := []struct {
		name     string
		input    string
		expected lexer.TokenType
	}{
		{
			name:     "Unterminated Template",
			input:    "`hello world",
			expected: lexer.ILLEGAL,
		},
		{
			name:     "Invalid Escape Sequence",
			input:    "`hello \\",
			expected: lexer.ILLEGAL,
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			l := lexer.NewLexer(tc.input)

			// Skip TEMPLATE_START
			l.NextToken()

			// Should get an error token
			tok := l.NextToken()
			if tok.Type != tc.expected {
				t.Errorf("Expected error token %s, got %s", tc.expected, tok.Type)
			}
		})
	}
}

// Test that template literals work correctly in different contexts
func TestTemplateLiteralContexts(t *testing.T) {
	contextCases := []struct {
		name  string
		input string
		// Just verify it tokenizes without crashing - full parsing will be tested later
	}{
		{
			name:  "Template in Variable Assignment",
			input: "let msg = `hello ${name}`;",
		},
		{
			name:  "Template in Function Call",
			input: "console.log(`result: ${x + y}`);",
		},
		{
			name:  "Template in Array",
			input: "let arr = [`first ${a}`, `second ${b}`];",
		},
		{
			name:  "Template in Object",
			input: "let obj = { msg: `hello ${user}` };",
		},
		{
			name:  "Multiple Templates",
			input: "`first` + `second ${x}` + `third`;",
		},
	}

	for _, tc := range contextCases {
		t.Run(tc.name, func(t *testing.T) {
			l := lexer.NewLexer(tc.input)

			// Just verify we can tokenize without errors
			for {
				tok := l.NextToken()
				if tok.Type == lexer.EOF {
					break
				}
				if tok.Type == lexer.ILLEGAL {
					t.Errorf("Unexpected ILLEGAL token: %q", tok.Literal)
					break
				}
			}
		})
	}
}
