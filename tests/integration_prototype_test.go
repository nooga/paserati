package tests

import (
	"testing"
	"paserati/pkg/driver"
	"paserati/pkg/vm"
)

// TestPrototypeRefactoredIntegration verifies the refactored property access works correctly
func TestPrototypeRefactoredIntegration(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected interface{}
	}{
		{
			name: "primitive_prototype_methods",
			code: `
// String prototype method
let str = "hello world";
let len = str.length;

// Array prototype method  
let arr = [1, 2, 3, 4, 5];
let arrLen = arr.length;

len + arrLen;`,
			expected: 16.0, // 11 + 5
		},
		{
			name: "function_prototype_access",
			code: `
function MyFunc() {
	this.value = 42;
}
MyFunc.prototype.getValue = function() {
	return this.value;
};

let instance = new MyFunc();
instance.getValue();`,
			expected: 42.0,
		},
		{
			name: "prototype_chain_traversal",
			code: `
function Animal() {}
Animal.prototype.species = "animal";
Animal.prototype.speak = function() {
	return "Some sound";
};

function Dog() {}
Dog.prototype = new Animal();
Dog.prototype.bark = function() {
	return "Woof!";
};

let dog = new Dog();
dog.species;`,
			expected: "animal",
		},
		// TODO: Enable when Function.prototype.call is implemented
		// {
		// 	name: "function_prototype_call", 
		// 	code: `
		// function multiply(x: number, y: number) {
		// 	return x * y;
		// }
		// 
		// multiply.call(null, 6, 7);`,
		// 	expected: 42.0,
		// },
		{
			name: "object_getPrototypeOf",
			code: `
function Person(name: string) {
	this.name = name;
}
Person.prototype.greet = function() {
	return "Hello, " + this.name;
};

let john = new Person("John");
let proto = Object.getPrototypeOf(john);
proto === Person.prototype;`,
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := driver.NewPaserati()
			result, errs := p.RunString(tc.code)
			if len(errs) > 0 {
				t.Fatalf("Evaluation failed: %v", errs)
			}

			// Check result based on expected type
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
			case bool:
				if !result.IsBoolean() {
					t.Errorf("Expected boolean, got %s", result.TypeName())
				} else if result.AsBoolean() != expected {
					t.Errorf("Expected %v, got %v", expected, result.AsBoolean())
				}
			default:
				t.Errorf("Unexpected type for expected value: %T", expected)
			}
		})
	}
}