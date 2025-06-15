package tests

import (
	"paserati/pkg/driver"
	"strings"
	"testing"
)

func TestWhileStatement(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // Expected final value output
		isError  bool   // If expected is runtime error substring
	}{
		{
			name: "Simple while loop",
			input: `
				let i = 0;
				let result = 0;
				while (i < 3) {
					result = result + i;
					i = i + 1;
				}
				result; // Expect 0 + 1 + 2 = 3
			`,
			expected: "3",
		},
		{
			name: "While loop condition initially false",
			input: `
				let result = 100;
				while (false) {
					result = 0;
				}
				result; // Should remain 100
			`,
			expected: "100",
		},
		{
			name: "While loop using variable condition",
			input: `
				let condition = true;
				let count = 0;
				while (condition) {
					if (count == 1) {
						condition = false; // Stop the loop
					}
					count = count + 1;
				}
				count; // Should execute twice (0, 1), ends with count = 2
			`,
			expected: "2",
		},
		{
			name: "While loop with break",
			input: `
				let i = 0;
				let result = 0;
				while(i < 5) {
					if (i == 2) {
						break;
					}
					result = result + i;
					i = i + 1;
				}
				result; // Should be 0 + 1 = 1
			`,
			expected: "1",
		},
		{
			name: "While loop with continue",
			input: `
				let i = 0;
				let result = 0;
				while(i < 4) {
					i = i + 1;
					if (i == 2) {
						continue;
					}
					result = result + i; // Add 1, 3, 4 (skips 2)
				}
				result; // Should be 1 + 3 + 4 = 8
			`,
			expected: "8",
		},
		// TODO: Add tests for break/continue later if implemented
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compile
			chunk, compileErrs := driver.CompileString(tt.input)
			if len(compileErrs) > 0 {
				var errMsgs strings.Builder
				for _, cerr := range compileErrs {
					errMsgs.WriteString(cerr.Error() + "\n")
				}
				t.Fatalf("Unexpected compile errors:\n%s", errMsgs.String())
			}
			if chunk == nil {
				t.Fatalf("Compilation succeeded but returned nil chunk")
			}

			// Run VM
			paserati := driver.NewPaserati()
			finalValue, runtimeErrs := paserati.InterpretChunk(chunk)

			// Check results
			if tt.isError {
				if len(runtimeErrs) == 0 {
					t.Errorf("Expected runtime error containing %q, but VM returned OK. Final Value: %s", tt.expected, finalValue.ToString())
				} else {
					found := false
					var allErrors strings.Builder
					for _, rerr := range runtimeErrs {
						allErrors.WriteString(rerr.Error() + "\n")
						if strings.Contains(rerr.Error(), tt.expected) {
							found = true
						}
					}
					if !found {
						t.Errorf("Expected runtime error containing %q, but got errors:\n%s", tt.expected, allErrors.String())
					}
				}
			} else {
				if len(runtimeErrs) > 0 {
					var allErrors strings.Builder
					for _, rerr := range runtimeErrs {
						allErrors.WriteString(rerr.Error() + "\n")
					}
					t.Errorf("Expected value %q, but got runtime errors:\n%s", tt.expected, allErrors.String())
				} else {
					actualOutput := finalValue.ToString()
					if actualOutput != tt.expected {
						t.Errorf("Expected output=%q, got=%q", tt.expected, actualOutput)
					}
				}
			}
		})
	}
}

func TestForStatement(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		isError  bool
	}{
		{
			name: "Simple for loop with let initializer",
			input: `
				let result = 0;
				for (let i = 0; i < 5; i = i + 1) {
					result = result + i;
				}
				result; // Expect 0 + 1 + 2 + 3 + 4 = 10
			`,
			expected: "10",
		},
		{
			name: "For loop with existing variable initializer",
			input: `
				let sum = 0;
				let i = 0; 
				for (i = 1; i < 4; i = i + 1) { // Initialize uses existing var
					sum = sum + i;
				}
				sum; // Expect 1 + 2 + 3 = 6
			`,
			expected: "6",
		},
		{
			name: "For loop with optional init and update",
			input: `
				let i = 0;
				let sum = 0;
				for (; i < 3; ) { // Optional init and update
					sum = sum + i;
					i = i + 1; // Update inside body
				}
				sum; // Expect 0 + 1 + 2 = 3
			`,
			expected: "3",
		},
		{
			name: "For loop condition initially false",
			input: `
                let result = 50;
                for(let i=0; false; i=i+1) {
                    result = 0;
                }
                result; // Should remain 50
            `,
			expected: "50",
		},
		{
			name: "For loop with no condition, using break",
			input: `
				let x = 0;
				for (;;) { 
					x = x + 1;
					if (x == 2) {
						break;
					}
				}
				x; // Expect 2
			`,
			expected: "2",
		},
		{
			name: "For loop with break",
			input: `
				let result = 0;
				for (let i=0; i < 10; i=i+1) {
					if (i == 3) {
						break;
					}
					result = result + i;
				}
				result; // Expect 0 + 1 + 2 = 3
			`,
			expected: "3",
		},
		{
			name: "For loop with continue",
			input: `
				let result = 0;
				for (let i=0; i < 5; i=i+1) {
					if (i == 2 || i == 4) {
						continue;
					}
					result = result + i; // Add 0, 1, 3
				}
				result; // Expect 0 + 1 + 3 = 4
			`,
			expected: "4",
		},
		{
			name: "For loop simple continue",
			input: `
				let result = 0;
				for (let i=0; i < 5; i=i+1) {
					if (i == 2) {
						continue;
					}
					result = result + i; // Add 0, 1, 3, 4
				}
				result; // Expect 0 + 1 + 3 + 4 = 8
			`,
			expected: "8",
		},
		// TODO: Add tests for nested loops, continue, etc. later
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compile
			chunk, compileErrs := driver.CompileString(tt.input)
			if len(compileErrs) > 0 {
				var errMsgs strings.Builder
				for _, cerr := range compileErrs {
					errMsgs.WriteString(cerr.Error() + "\n")
				}
				t.Fatalf("Unexpected compile errors:\n%s", errMsgs.String())
			}
			if chunk == nil {
				t.Fatalf("Compilation succeeded but returned nil chunk")
			}

			// Run VM
			paserati := driver.NewPaserati()
			finalValue, runtimeErrs := paserati.InterpretChunk(chunk)

			// Check results
			if tt.isError {
				if len(runtimeErrs) == 0 {
					t.Errorf("Expected runtime error containing %q, but VM returned OK. Final Value: %s", tt.expected, finalValue.ToString())
				} else {
					found := false
					var allErrors strings.Builder
					for _, rerr := range runtimeErrs {
						allErrors.WriteString(rerr.Error() + "\n")
						if strings.Contains(rerr.Error(), tt.expected) {
							found = true
						}
					}
					if !found {
						t.Errorf("Expected runtime error containing %q, but got errors:\n%s", tt.expected, allErrors.String())
					}
				}
			} else {
				if len(runtimeErrs) > 0 {
					var allErrors strings.Builder
					for _, rerr := range runtimeErrs {
						allErrors.WriteString(rerr.Error() + "\n")
					}
					t.Errorf("Expected value %q, but got runtime errors:\n%s", tt.expected, allErrors.String())
				} else {
					actualOutput := finalValue.ToString()
					if actualOutput != tt.expected {
						t.Errorf("Expected output=%q, got=%q", tt.expected, actualOutput)
					}
				}
			}
		})
	}
}

func TestDoWhileStatement(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // Expected final value output
		isError  bool
	}{
		{
			name: "Simple do-while loop",
			input: `
				let i = 0;
				let result = 0;
				do {
					result = result + i;
					i = i + 1;
				} while (i < 3);
				result; // Expect 0 + 1 + 2 = 3
			`,
			expected: "3",
		},
		{
			name: "Do-while loop condition initially false",
			input: `
				let result = 100;
				let i = 0;
				do {
					result = 50; // Body executes once
					i = 1;
				} while (i < 1); // Condition is false after first run
				result; // Should be 50
			`,
			expected: "50",
		},
		{
			name: "Do-while loop with break",
			input: `
				let i = 0;
				let result = 0;
				do {
					if (i == 2) {
						break;
					}
					result = result + i;
					i = i + 1;
				} while(i < 5);
				result; // Should be 0 + 1 = 1
			`,
			expected: "1",
		},
		{
			name: "Do-while loop with continue",
			input: `
				let i = 0;
				let result = 0;
				do {
					i = i + 1;
					if (i == 2) {
						continue; // Skip result = result + i when i=2
					}
					result = result + i; // Add 1, 3, 4
				} while(i < 4);
				result; // Should be 1 + 3 + 4 = 8
			`,
			expected: "8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compile
			chunk, compileErrs := driver.CompileString(tt.input)
			if len(compileErrs) > 0 {
				var errMsgs strings.Builder
				for _, cerr := range compileErrs {
					errMsgs.WriteString(cerr.Error() + "\n")
				}
				t.Fatalf("Unexpected compile errors:\n%s", errMsgs.String())
			}
			if chunk == nil {
				t.Fatalf("Compilation succeeded but returned nil chunk")
			}

			// Run VM
			paserati := driver.NewPaserati()
			finalValue, runtimeErrs := paserati.InterpretChunk(chunk)

			// Check results
			if tt.isError {
				if len(runtimeErrs) == 0 {
					t.Errorf("Expected runtime error containing %q, but VM returned OK. Final Value: %s", tt.expected, finalValue.ToString())
				} else {
					found := false
					var allErrors strings.Builder
					for _, rerr := range runtimeErrs {
						allErrors.WriteString(rerr.Error() + "\n")
						if strings.Contains(rerr.Error(), tt.expected) {
							found = true
						}
					}
					if !found {
						t.Errorf("Expected runtime error containing %q, but got errors:\n%s", tt.expected, allErrors.String())
					}
				}
			} else {
				if len(runtimeErrs) > 0 {
					var allErrors strings.Builder
					for _, rerr := range runtimeErrs {
						allErrors.WriteString(rerr.Error() + "\n")
					}
					t.Errorf("Expected value %q, but got runtime errors:\n%s", tt.expected, allErrors.String())
				} else {
					actualOutput := finalValue.ToString()
					if actualOutput != tt.expected {
						t.Errorf("Expected output=%q, got=%q", tt.expected, actualOutput)
					}
				}
			}
		})
	}
}
