package tests

import (
	"paserati/pkg/driver"
	"strings"
	"testing"
)

func TestNewKeyword(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name:     "Basic constructor",
			code:     `function Test() {} new Test()`,
			expected: "{}",
		},
		{
			name:     "Constructor with primitive return",
			code:     `function Test() { if (true) { return 42; } } new Test()`,
			expected: "{}", // Should return instance, not primitive
		},
		{
			name:     "Constructor with object return",
			code:     `function Test() { if (true) { return {}; } } new Test()`,
			expected: "{}", // Should return the explicit object
		},
		{
			name:     "Constructor with no return",
			code:     `function Test() { let x = 1; } new Test()`,
			expected: "{}",
		},
		{
			name:     "Constructor with arguments",
			code:     `function Test(a, b) {} new Test(1, 2)`,
			expected: "{}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compile the code
			chunk, compileErrs := driver.CompileString(tt.code)
			if len(compileErrs) > 0 {
				var allErrors strings.Builder
				for _, cerr := range compileErrs {
					allErrors.WriteString(cerr.Error() + "\n")
				}
				t.Fatalf("Unexpected compile errors:\n%s", allErrors.String())
			}
			if chunk == nil {
				t.Fatalf("Compilation succeeded but returned a nil chunk unexpectedly.")
			}

			// Run the code
			paserati := driver.NewPaserati()
			finalValue, runtimeErrs := paserati.InterpretChunk(chunk)
			if len(runtimeErrs) > 0 {
				var allErrors strings.Builder
				for _, rerr := range runtimeErrs {
					allErrors.WriteString(rerr.Error() + "\n")
				}
				t.Fatalf("Unexpected runtime errors:\n%s", allErrors.String())
			}

			// Check the result - use Inspect() instead of ToString()
			actualOutput := finalValue.Inspect()
			if actualOutput != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, actualOutput)
			}
		})
	}
}

func TestNewKeywordErrors(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{
			name: "New with non-function",
			code: `let x = 42; new x()`,
		},
		{
			name: "New with undefined",
			code: `new undefined()`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compile the code
			chunk, compileErrs := driver.CompileString(tt.code)
			if len(compileErrs) > 0 {
				// Expected compile error for some cases
				return
			}
			if chunk == nil {
				t.Fatalf("Compilation succeeded but returned a nil chunk unexpectedly.")
			}

			// Run the code
			paserati := driver.NewPaserati()
			_, runtimeErrs := paserati.InterpretChunk(chunk)
			if len(runtimeErrs) == 0 {
				t.Errorf("Expected error for: %s", tt.code)
			}
		})
	}
}
