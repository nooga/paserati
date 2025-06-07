package tests

import (
	"paserati/pkg/driver"
	"paserati/pkg/vm"
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
			// 1. Compile using the driver
			chunk, compileErrs := driver.CompileString(tc.input)

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

			// 2. Run VM
			vmInstance := vm.NewVM()
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
