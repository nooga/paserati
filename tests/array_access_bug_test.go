package tests

import (
	"testing"

	"paserati/pkg/driver"
)

// Test for the array access bug where Array(1, 2, 3)[2] returns 2 instead of 3
// in complex expressions involving clock() and Array(10).length
func TestArrayAccessBug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected interface{}
	}{
		{
			name:     "Array access alone - should work",
			input:    "Array(1, 2, 3)[2];",
			expected: 3.0,
		},
		{
			name:     "Array access with equality - should work",
			input:    "Array(1, 2, 3)[2] == 3;",
			expected: true,
		},
		{
			name:     "Array length check alone - should work",
			input:    "Array(10).length == 10;",
			expected: true,
		},
		{
			name:     "Two array operations together - should work",
			input:    "Array(10).length == 10 && Array(1, 2, 3)[2] == 3;",
			expected: true,
		},
		{
			name:     "Clock with array access - should work",
			input:    "clock() > 0 && Array(1, 2, 3)[2] == 3;",
			expected: true,
		},
		{
			name:     "Full expression - currently broken",
			input:    "clock() > 0 && Array(10).length == 10 && Array(1, 2, 3)[2] == 3;",
			expected: true,
		},
		{
			name:     "Just the array value in full context",
			input:    "clock() > 0 && Array(10).length == 10 && Array(1, 2, 3)[2];",
			expected: 3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paserati := driver.NewPaserati()
			result, err := paserati.RunString(tt.input)
			if len(err) > 0 {
				t.Fatalf("Unexpected error: %v", err)
			}

			var expected interface{}
			switch v := tt.expected.(type) {
			case float64:
				expected = v
			case bool:
				expected = v
			}

			var actual interface{}
			if result.IsNumber() {
				actual = result.AsFloat()
			} else if result.IsBoolean() {
				actual = result.AsBoolean()
			}

			if actual != expected {
				t.Errorf("Expected %v, got %v", expected, actual)
			}
		})
	}
}
