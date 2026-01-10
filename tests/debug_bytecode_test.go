package tests

import (
	"testing"

	"github.com/nooga/paserati/pkg/driver"
)

func TestCompilerBytecode(t *testing.T) {
	// Simple method call that should work
	code := `
function Test() {
	this.value = 42;
}
Test.prototype.getValue = function() {
	return this.value;
};

let t = new Test();
let result = t.getValue();
result;`

	p := driver.NewPaserati()
	result, errs := p.RunString(code)
	if len(errs) > 0 {
		t.Fatalf("Test failed: %v", errs)
	}
	t.Logf("Method call result: %s = %v", result.TypeName(), result.Inspect())

	// Test direct property access
	code2 := `
function Test() {
	this.value = 42;
}
Test.prototype.getValue = function() {
	return this.value;
};

let t = new Test();
let method = t.getValue; // Just access, don't call
method;`

	p2 := driver.NewPaserati()
	result2, errs2 := p2.RunString(code2)
	if len(errs2) > 0 {
		t.Fatalf("Test2 failed: %v", errs2)
	}
	t.Logf("Property access result: %s = %v", result2.TypeName(), result2.Inspect())
}
