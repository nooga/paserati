package tests

import (
	"paserati/pkg/driver"
	"strings"
	"testing"
)

func TestRestParameterParsing(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"function test(...args) { return args; }",
			"function test(...args) { return args; }",
		},
		{
			"function test(...args: number[]) { return args; }",
			"function test(...args: number[]) { return args; }",
		},
		{
			"function test(a: number, ...rest: string[]) { return rest; }",
			"function test(a: number, ...rest: string[]) { return rest; }",
		},
		{
			"let arrow = (...items) => items;",
			"let arrow = (...items) => items;",
		},
	}

	for _, tt := range tests {
		// Just test that parsing doesn't fail - we expect type/compile errors for now
		// Use CompileString to test parsing and compilation without execution
		_, errs := driver.CompileString(tt.input)

		// We expect type errors since rest parameters aren't fully implemented yet
		// But we want to make sure it's not a parse error
		if len(errs) > 0 {
			for _, err := range errs {
				if strings.Contains(err.Error(), "Syntax Error") {
					t.Errorf("Unexpected syntax error for input %s: %v", tt.input, err)
				}
			}
		}
	}
}

func TestSpreadSyntaxParsing(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"console.log(...[1, 2, 3]);",
			"console.log(...[1, 2, 3]);",
		},
		{
			"let arr = [1, 2]; console.log(...arr);",
			"let arr = [1, 2]; console.log(...arr);",
		},
	}

	for _, tt := range tests {
		// Just test that parsing doesn't fail - we expect compilation issues for now
		// Use CompileString to test parsing and compilation without execution
		_, errs := driver.CompileString(tt.input)

		// We expect some issues since spread syntax isn't fully implemented yet
		// But we want to make sure it's not a parse error
		if len(errs) > 0 {
			for _, err := range errs {
				if strings.Contains(err.Error(), "Syntax Error") {
					t.Errorf("Unexpected syntax error for input %s: %v", tt.input, err)
				}
			}
		}
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
