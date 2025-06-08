package tests

import (
	"testing"
	"paserati/pkg/driver"
	"paserati/pkg/vm"
	"os"
)

func TestPrototypeCacheBasic(t *testing.T) {
	// Enable prototype caching for this test
	os.Setenv("PASERATI_ENABLE_PROTO_CACHE", "true")
	vm.EnablePrototypeCache = true
	defer func() {
		os.Unsetenv("PASERATI_ENABLE_PROTO_CACHE")
		vm.EnablePrototypeCache = false
	}()

	tests := []struct {
		name     string
		code     string
		expected interface{}
	}{
		{
			name: "string_prototype_method",
			code: `
let str = "hello";
str.length;`,
			expected: 5.0,
		},
		{
			name: "array_prototype_method",
			code: `
let arr = [1, 2, 3];
arr.length;`,
			expected: 3.0,
		},
		// TODO: Enable when Function.prototype.call is implemented
		// {
		// 	name: "function_prototype_call",
		// 	code: `
		// function greet(name: string) {
		// 	return "Hello, " + name;
		// }
		// greet.call(null, "World");`,
		// 	expected: "Hello, World",
		// },
		// TODO: Fix parser panic in this test
		// {
		// 	name: "prototype_chain_lookup", 
		// 	code: `
		// function Animal() {
		// 	this.type = "animal";
		// }
		// Animal.prototype.getType = function() {
		// 	return this.type;
		// };
		// 
		// let a = new Animal();
		// a.getType();`,
		// 	expected: "animal",
		// },
		{
			name: "deep_prototype_chain",
			code: `
function A() {}
A.prototype.value = 1;

function B() {}
B.prototype = new A();

function C() {}
C.prototype = new B();

let c = new C();
c.value;`,
			expected: 1.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := driver.NewPaserati()
			result, errs := p.RunString(tc.code)
			if len(errs) > 0 {
				t.Fatalf("Evaluation failed: %v", errs)
			}

			// Check result based on type
			switch expected := tc.expected.(type) {
			case float64:
				if !result.IsNumber() {
					t.Errorf("Expected number, got %s", result.TypeName())
				} else if vm.AsNumber(result) != expected {
					t.Errorf("Expected %v, got %v", expected, vm.AsNumber(result))
				}
			case string:
				if !result.IsString() {
					t.Errorf("Expected string, got %s", result.TypeName())
				} else if vm.AsString(result) != expected {
					t.Errorf("Expected %q, got %q", expected, vm.AsString(result))
				}
			default:
				t.Errorf("Unexpected type for expected value: %T", expected)
			}
		})
	}
}

func TestPrototypeCachePerformance(t *testing.T) {
	// Compare performance with and without prototype caching
	code := `
function Base() {
	this.value = 42;
}
Base.prototype.getValue = function() {
	return this.value;
};

let obj = new Base();
obj.getValue();`

	// Test without caching
	os.Setenv("PASERATI_ENABLE_PROTO_CACHE", "false")
	vm.EnablePrototypeCache = false
	vm.ResetExtendedStats()
	
	p1 := driver.NewPaserati()
	result1, errs1 := p1.RunString(code)
	if len(errs1) > 0 {
		t.Fatalf("Evaluation without cache failed: %v", errs1)
	}

	// Test with caching
	os.Setenv("PASERATI_ENABLE_PROTO_CACHE", "true")
	vm.EnablePrototypeCache = true
	vm.ResetExtendedStats()
	
	p2 := driver.NewPaserati()
	result2, errs2 := p2.RunString(code)
	if len(errs2) > 0 {
		t.Fatalf("Evaluation with cache failed: %v", errs2)
	}

	// Results should be the same
	if result1 != result2 {
		t.Errorf("Results differ: without cache = %v, with cache = %v", result1, result2)
	}

	// Expected result is 42
	expectedSum := 42.0
	if !result1.IsNumber() || vm.AsNumber(result1) != expectedSum {
		t.Errorf("Expected sum %v, got %v", expectedSum, vm.AsNumber(result1))
	}
}