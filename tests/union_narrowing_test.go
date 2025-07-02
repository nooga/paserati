package tests

import (
	"fmt"
	"paserati/pkg/builtins"
	"paserati/pkg/compiler"
	"paserati/pkg/errors"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
	"paserati/pkg/vm"
	"sort"
	"strings"
	"testing"
)

type unionNarrowingTestCase struct {
	name               string
	input              string
	expect             string // Expected output OR expected error substring
	isError            bool   // True if expect is a runtime error substring
	expectCompileError bool   // True if expect is a compile error substring
}

func TestUnionTypeNarrowing(t *testing.T) {
	tests := []unionNarrowingTestCase{
		{
			name: "String narrowing in then branch",
			input: `
				let x: string | number = "hello";
				let result;
				if (typeof x === "string") {
					result = x.length;
				} else {
					result = x * 2;
				}
				result;
			`,
			expect: "5",
		},
		{
			name: "Number narrowing in else branch",
			input: `
				let x: string | number = 42;
				let result;
				if (typeof x === "string") {
					result = x.length;
				} else {
					result = x * 2;
				}
				result;
			`,
			expect: "84",
		},
		{
			name: "Three-way union narrowing",
			input: `
				let x: string | number | boolean = "test";
				let result;
				if (typeof x === "string") {
					result = x.toUpperCase();
				} else {
					result = typeof x;
				}
				result;
			`,
			expect: "TEST",
		},
		{
			name: "Three-way union else branch",
			input: `
				let x: string | number | boolean = 42;
				let result;
				if (typeof x === "string") {
					result = x.toUpperCase();
				} else {
					result = typeof x;
				}
				result;
			`,
			expect: "number",
		},
		{
			name: "Sequential narrowing",
			input: `
				let x: string | number = 42;
				let result;
				if (typeof x === "string") {
					result = "string";
				} else if (typeof x === "number") {
					result = x + 10;
				} else {
					result = "something else";
				}
				result;
			`,
			expect: "52",
		},
		{
			name: "Function parameter narrowing",
			input: `
				function processValue(x: string | number): number {
					if (typeof x === "string") {
						return x.length;
					} else {
						return x * 2;
					}
				}
				processValue("hello") + processValue(21);
			`,
			expect: "47",
		},
		{
			name: "Invalid narrowing attempt",
			input: `
				let x: string | number = "test";
				let result;
				if (typeof x === "boolean") {
					result = "boolean";
				} else {
					result = "not boolean";
				}
				result;
			`,
			expect: "not boolean",
		},
		{
			name: "Unknown type narrowing still works",
			input: `
				let x: unknown = "hello";
				let result;
				if (typeof x === "string") {
					result = x.length;
				} else {
					result = "not a string";
				}
				result;
			`,
			expect: "5",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// 1. Use coordinated compilation and VM initialization like scripts_test.go
			chunk, vmInstance, compileErrs := compileAndInitializeVMFromString(tc.input)

			// Handle expected compile errors
			if tc.expectCompileError {
				if len(compileErrs) == 0 {
					t.Fatalf("Expected compile error containing %q, but got no errors.", tc.expect)
				}
				found := false
				var allErrors strings.Builder
				for _, cerr := range compileErrs {
					allErrors.WriteString(cerr.Error() + "\n")
					if strings.Contains(cerr.Error(), tc.expect) {
						found = true
					}
				}
				if !found {
					t.Errorf("Expected compile error containing %q, but got errors:\n%s", tc.expect, allErrors.String())
				}
				return // Test passes if expected compile error is found
			}

			// Handle unexpected compile errors
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

			// 2. Run VM (already initialized with coordinated globals)
			finalValue, runtimeErrs := vmInstance.Interpret(chunk)

			// 3. Check Results
			if tc.isError {
				if len(runtimeErrs) == 0 {
					t.Errorf("Expected runtime error containing %q, but VM returned OK. Final Value: %s", tc.expect, finalValue.ToString())
				} else {
					found := false
					var allErrors strings.Builder
					for _, rerr := range runtimeErrs {
						allErrors.WriteString(rerr.Error() + "\n")
						if strings.Contains(rerr.Error(), tc.expect) {
							found = true
						}
					}
					if !found {
						t.Errorf("Expected runtime error containing %q, but got errors:\n%s", tc.expect, allErrors.String())
					}
				}
			} else {
				if len(runtimeErrs) > 0 {
					var allErrors strings.Builder
					for _, rerr := range runtimeErrs {
						allErrors.WriteString(rerr.Error() + "\n")
					}
					t.Errorf("Expected value %q, but got runtime errors:\n%s", tc.expect, allErrors.String())
				} else {
					actualOutput := finalValue.ToString()
					if actualOutput != tc.expect {
						t.Errorf("Test %s failed.\nInput:    %q\nExpected: %q\nGot:      %q", tc.name, tc.input, tc.expect, actualOutput)
					}
				}
			}
		})
	}
}

// compileAndInitializeVMFromString compiles a string and creates a VM with coordinated global indices
// This is similar to compileAndInitializeVM from scripts_test.go but works with strings instead of files
func compileAndInitializeVMFromString(source string) (*vm.Chunk, *vm.VM, []errors.PaseratiError) {
	// Parse
	l := lexer.NewLexer(source)
	p := parser.NewParser(l)
	program, parseErrs := p.ParseProgram()
	if len(parseErrs) > 0 {
		return nil, nil, parseErrs
	}

	// Create compiler and VM
	comp := compiler.NewCompiler()
	vmInstance := vm.NewVM()

	// Get all standard initializers for coordination
	initializers := builtins.GetStandardInitializers()
	sort.Slice(initializers, func(i, j int) bool {
		return initializers[i].Priority() < initializers[j].Priority()
	})

	// Initialize runtime context
	globalVariables := make(map[string]vm.Value)
	runtimeCtx := &builtins.RuntimeContext{
		VM: vmInstance,
		DefineGlobal: func(name string, value vm.Value) error {
			globalVariables[name] = value
			return nil
		},
	}

	// Initialize all builtins runtime values
	for _, init := range initializers {
		if err := init.InitRuntime(runtimeCtx); err != nil {
			compileErr := &errors.CompileError{
				Position: errors.Position{Line: 0, Column: 0},
				Msg:      fmt.Sprintf("Failed to initialize %s runtime: %v", init.Name(), err),
			}
			return nil, nil, []errors.PaseratiError{compileErr}
		}
	}

	// Pre-populate compiler global indices in alphabetical order to match VM
	var globalNames []string
	for name := range globalVariables {
		globalNames = append(globalNames, name)
	}
	sort.Strings(globalNames)

	// Pre-assign global indices in the compiler to match VM ordering
	for _, name := range globalNames {
		comp.GetOrAssignGlobalIndex(name)
	}

	// Set up global variables in VM with empty index map (legacy test mode)
	indexMap := make(map[string]int)
	if err := vmInstance.SetBuiltinGlobals(globalVariables, indexMap); err != nil {
		compileErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Failed to set VM globals: %v", err),
		}
		return nil, nil, []errors.PaseratiError{compileErr}
	}

	// Compile
	chunk, compileAndTypeErrs := comp.Compile(program)
	if len(compileAndTypeErrs) > 0 {
		return nil, vmInstance, compileAndTypeErrs
	}

	return chunk, vmInstance, nil
}
