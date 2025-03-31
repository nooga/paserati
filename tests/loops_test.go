package tests

import (
	"bytes"
	"os"
	"paserati/pkg/driver"
	"paserati/pkg/vm"
	"strings"
	"testing"
)

func TestWhileStatement(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // Expected final value output (stdout)
		isError  bool
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
			chunk, compileErrs := driver.CompileString(tt.input)
			if len(compileErrs) > 0 {
				t.Fatalf("Unexpected compile errors: %v", compileErrs)
			}
			if chunk == nil {
				t.Fatalf("Compilation succeeded but returned nil chunk")
			}

			vmInstance := vm.NewVM()
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			oldStderr := os.Stderr
			rErr, wErr, _ := os.Pipe()
			os.Stderr = wErr

			interpretResult := vmInstance.Interpret(chunk)

			w.Close()
			os.Stdout = oldStdout
			wErr.Close()
			os.Stderr = oldStderr

			var vmStdout bytes.Buffer
			_, _ = vmStdout.ReadFrom(r)
			actualOutput := strings.TrimSpace(vmStdout.String())

			var vmStderr bytes.Buffer
			_, _ = vmStderr.ReadFrom(rErr)
			actualRuntimeError := strings.TrimSpace(vmStderr.String())

			if tt.isError {
				if interpretResult == vm.InterpretOK {
					t.Errorf("Expected runtime error containing %q, but VM returned OK. Stdout: %q", tt.expected, actualOutput)
				} else if !strings.Contains(actualRuntimeError, tt.expected) {
					t.Errorf("Expected runtime error containing %q, but got stderr: %q", tt.expected, actualRuntimeError)
				}
			} else {
				if interpretResult != vm.InterpretOK {
					t.Errorf("Expected VM OK, but got %v. Stderr: %q", interpretResult, actualRuntimeError)
				}
				if actualOutput != tt.expected {
					t.Errorf("Expected output=%q, got=%q", tt.expected, actualOutput)
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
		// TODO: Add tests for nested loops, continue, etc. later
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunk, compileErrs := driver.CompileString(tt.input)
			if len(compileErrs) > 0 {
				t.Fatalf("Unexpected compile errors: %v", compileErrs)
			}
			if chunk == nil {
				t.Fatalf("Compilation succeeded but returned nil chunk")
			}

			vmInstance := vm.NewVM()
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			oldStderr := os.Stderr
			rErr, wErr, _ := os.Pipe()
			os.Stderr = wErr

			interpretResult := vmInstance.Interpret(chunk)

			w.Close()
			os.Stdout = oldStdout
			wErr.Close()
			os.Stderr = oldStderr

			var vmStdout bytes.Buffer
			_, _ = vmStdout.ReadFrom(r)
			actualOutput := strings.TrimSpace(vmStdout.String())

			var vmStderr bytes.Buffer
			_, _ = vmStderr.ReadFrom(rErr)
			actualRuntimeError := strings.TrimSpace(vmStderr.String())

			if tt.isError {
				if interpretResult == vm.InterpretOK {
					t.Errorf("Expected runtime error containing %q, but VM returned OK. Stdout: %q", tt.expected, actualOutput)
				} else if !strings.Contains(actualRuntimeError, tt.expected) {
					t.Errorf("Expected runtime error containing %q, but got stderr: %q", tt.expected, actualRuntimeError)
				}
			} else {
				if interpretResult != vm.InterpretOK {
					t.Errorf("Expected VM OK, but got %v. Stderr: %q", interpretResult, actualRuntimeError)
				}
				if actualOutput != tt.expected {
					t.Errorf("Expected output=%q, got=%q", tt.expected, actualOutput)
				}
			}
		})
	}
}
