package tests

import (
	"testing"

	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
)

const templateParserDebug = false // Enable for debugging

func TestTemplateLiteralParsing(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		validate func(t *testing.T, program *parser.Program)
	}{
		{
			name:  "Simple Template",
			input: "`hello world`",
			validate: func(t *testing.T, program *parser.Program) {
				validateProgramLength(t, program, 1)

				stmt := program.Statements[0].(*parser.ExpressionStatement)
				tl := stmt.Expression.(*parser.TemplateLiteral)

				if len(tl.Parts) != 1 {
					t.Errorf("Expected 1 part, got %d", len(tl.Parts))
				}

				part := tl.Parts[0].(*parser.TemplateStringPart)
				if part.Value != "hello world" {
					t.Errorf("Expected 'hello world', got '%s'", part.Value)
				}
			},
		},
		{
			name:  "Empty Template",
			input: "``",
			validate: func(t *testing.T, program *parser.Program) {
				validateProgramLength(t, program, 1)

				stmt := program.Statements[0].(*parser.ExpressionStatement)
				tl := stmt.Expression.(*parser.TemplateLiteral)

				if len(tl.Parts) != 1 {
					t.Errorf("Expected 1 part, got %d", len(tl.Parts))
				}

				part := tl.Parts[0].(*parser.TemplateStringPart)
				if part.Value != "" {
					t.Errorf("Expected empty string, got '%s'", part.Value)
				}
			},
		},
		{
			name:  "Simple Interpolation",
			input: "`hello ${name}`",
			validate: func(t *testing.T, program *parser.Program) {
				validateProgramLength(t, program, 1)

				stmt := program.Statements[0].(*parser.ExpressionStatement)
				tl := stmt.Expression.(*parser.TemplateLiteral)

				if len(tl.Parts) != 3 {
					t.Errorf("Expected 3 parts, got %d", len(tl.Parts))
				}

				// First part: "hello "
				part1 := tl.Parts[0].(*parser.TemplateStringPart)
				if part1.Value != "hello " {
					t.Errorf("Expected 'hello ', got '%s'", part1.Value)
				}

				// Second part: identifier "name"
				part2 := tl.Parts[1].(*parser.Identifier)
				if part2.Value != "name" {
					t.Errorf("Expected 'name', got '%s'", part2.Value)
				}

				// Third part: empty string
				part3 := tl.Parts[2].(*parser.TemplateStringPart)
				if part3.Value != "" {
					t.Errorf("Expected empty string, got '%s'", part3.Value)
				}
			},
		},
		{
			name:  "Multiple Interpolations",
			input: "`${x} + ${y} = ${result}`",
			validate: func(t *testing.T, program *parser.Program) {
				validateProgramLength(t, program, 1)

				stmt := program.Statements[0].(*parser.ExpressionStatement)
				tl := stmt.Expression.(*parser.TemplateLiteral)

				if len(tl.Parts) != 7 {
					t.Errorf("Expected 7 parts, got %d", len(tl.Parts))
				}

				// Check alternating pattern: string, expr, string, expr, string, expr, string
				expectedParts := []interface{}{
					"",       // Empty string before first interpolation
					"x",      // First variable
					" + ",    // String between interpolations
					"y",      // Second variable
					" = ",    // String between interpolations
					"result", // Third variable
					"",       // Empty string after last interpolation
				}

				for i, expected := range expectedParts {
					if i%2 == 0 {
						// String part
						part := tl.Parts[i].(*parser.TemplateStringPart)
						if part.Value != expected.(string) {
							t.Errorf("Part %d: expected '%s', got '%s'", i, expected.(string), part.Value)
						}
					} else {
						// Expression part
						part := tl.Parts[i].(*parser.Identifier)
						if part.Value != expected.(string) {
							t.Errorf("Part %d: expected '%s', got '%s'", i, expected.(string), part.Value)
						}
					}
				}
			},
		},
		{
			name:  "Template in Variable Assignment",
			input: "let msg = `hello ${name}`;",
			validate: func(t *testing.T, program *parser.Program) {
				validateProgramLength(t, program, 1)

				stmt := program.Statements[0].(*parser.LetStatement)
				if stmt.Name.Value != "msg" {
					t.Errorf("Expected variable name 'msg', got '%s'", stmt.Name.Value)
				}

				tl := stmt.Value.(*parser.TemplateLiteral)
				if len(tl.Parts) != 3 {
					t.Errorf("Expected 3 parts, got %d", len(tl.Parts))
				}
			},
		},
		{
			name:  "Only Interpolations",
			input: "`${a}${b}${c}`",
			validate: func(t *testing.T, program *parser.Program) {
				validateProgramLength(t, program, 1)

				stmt := program.Statements[0].(*parser.ExpressionStatement)
				tl := stmt.Expression.(*parser.TemplateLiteral)

				if len(tl.Parts) != 7 {
					t.Errorf("Expected 7 parts, got %d", len(tl.Parts))
				}

				// Should be: "", "a", "", "b", "", "c", ""
				expectedVars := []string{"a", "b", "c"}
				for i := 0; i < 7; i++ {
					if i%2 == 0 {
						// String part - should be empty
						part := tl.Parts[i].(*parser.TemplateStringPart)
						if part.Value != "" {
							t.Errorf("Part %d: expected empty string, got '%s'", i, part.Value)
						}
					} else {
						// Expression part
						part := tl.Parts[i].(*parser.Identifier)
						expectedVar := expectedVars[i/2]
						if part.Value != expectedVar {
							t.Errorf("Part %d: expected '%s', got '%s'", i, expectedVar, part.Value)
						}
					}
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if templateParserDebug {
				t.Logf("Testing: %s", tc.input)
			}

			l := lexer.NewLexer(tc.input)
			p := parser.NewParser(l)
			program, errors := p.ParseProgram()

			if len(errors) > 0 {
				t.Fatalf("Parser errors: %v", errors)
			}

			if program == nil {
				t.Fatalf("ParseProgram() returned nil")
			}

			tc.validate(t, program)
		})
	}
}

// Helper function to validate program length
func validateProgramLength(t *testing.T, program *parser.Program, expected int) {
	if len(program.Statements) != expected {
		t.Fatalf("Expected %d statements, got %d", expected, len(program.Statements))
	}
}
