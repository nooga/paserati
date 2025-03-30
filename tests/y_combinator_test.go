package tests

import (
	"bytes"
	// "io/ioutil"
	"os"
	"paserati/pkg/compiler"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
	"paserati/pkg/vm"

	// "path/filepath" // No longer needed
	"strings"
	"testing"
)

// compileSource compiles the given source string and handles errors.
// Uses testing.TB for compatibility with both tests and benchmarks.
func compileSource(tb testing.TB, source string) (*vm.Chunk, bool) {
	tb.Helper()

	l := lexer.NewLexer(source)
	p := parser.NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		for _, msg := range p.Errors() {
			tb.Logf("Parse Error: %s", msg) // Log errors for visibility
		}
		tb.Errorf("Parser encountered %d errors", len(p.Errors()))
		return nil, false // Indicate failure
	}

	comp := compiler.NewCompiler()
	chunk, compileErrs := comp.Compile(program)
	if len(compileErrs) > 0 {
		for _, msg := range compileErrs {
			tb.Logf("Compile Error: %s", msg)
		}
		tb.Errorf("Compiler encountered %d errors", len(compileErrs))
		return nil, false // Indicate failure
	}
	return chunk, true // Indicate success
}

// TestArrowFunctionSyntax compiles and runs various arrow function syntaxes,
// checking if the final result (printed to stdout) matches expectations.
func TestArrowFunctionSyntax(t *testing.T) {
	testCases := []struct {
		name           string
		script         string
		expectedOutput string
	}{
		{
			name:           "Simple Single Param",
			script:         `const double = x => x * 2; double(10);`,
			expectedOutput: "20",
		},
		// Add more test cases here
		{
			name:           "No Params",
			script:         `const fortyTwo = () => 42; fortyTwo();`,
			expectedOutput: "42",
		},
		{
			name:           "Single Param Parens",
			script:         `const increment = (y) => y + 1; increment(5);`,
			expectedOutput: "6",
		},
		{
			name:           "Multi Params",
			script:         `const subtract = (a, b) => a - b; subtract(10, 3);`,
			expectedOutput: "7",
		},
		{
			name:           "Block Body",
			script:         `const square = z => { let result = z * z; return result; }; square(4);`,
			expectedOutput: "16",
		},
		{
			name:           "Nested Simple",
			script:         `const add = x => y => x + y; const add5 = add(5); add5(3);`,
			expectedOutput: "8",
		},
		{
			name: "Y Combinator Factorial",
			script: `// The Y Combinator
const Y = (f) => ((x) => f((y) => x(x)(y)))((x) => f((y) => x(x)(y)));

// Factorial function generator (using if and ==)
const FactGen = f => n => {
  if (n == 0) {
    return 1;
  }
  // Implicit else
  return n * f(n - 1);
};

// Create the factorial function using the Y Combinator
const factorial = Y(FactGen);

// Calculate factorial of 5
factorial(5); // Should result in 120`,
			expectedOutput: "120",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			chunk, ok := compileSource(t, tc.script)
			if !ok {
				// compileSource already logged errors and called t.Errorf
				return
			}

			vmInstance := vm.NewVM()

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Run the VM
			resultStatus := vmInstance.Interpret(chunk)

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Check VM status first
			if resultStatus != vm.InterpretOK {
				t.Fatalf("VM interpretation did not return InterpretOK, got: %v", resultStatus)
			}

			// Read captured output
			var capturedOutput bytes.Buffer
			_, _ = capturedOutput.ReadFrom(r)
			actualOutput := capturedOutput.String()
			trimmedActualOutput := strings.TrimSpace(actualOutput)

			// Compare captured output with expected output
			if trimmedActualOutput != tc.expectedOutput {
				t.Errorf("Expected stdout %q, but got %q (Full captured: %q)",
					tc.expectedOutput, trimmedActualOutput, actualOutput)
			}
		})
	}
}
