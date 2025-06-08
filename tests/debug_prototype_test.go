package tests

import (
	"testing"
	"paserati/pkg/driver"
)

func TestPrototypeMethodBinding(t *testing.T) {
	// Test if the method is properly bound to 'this'
	code := `
function Test() {
	this.value = 42;
}
Test.prototype.getValue = function() {
	return this.value;
};

let t = new Test();

// Check what 'this' is when calling the method
Test.prototype.getValue.call(t);`

	p := driver.NewPaserati()
	result, errs := p.RunString(code)
	if len(errs) > 0 {
		t.Fatalf("Test failed: %v", errs)
	}
	t.Logf("Manual prototype call: %s = %v", result.TypeName(), result.Inspect())
}

func TestPrototypeMethodLookup(t *testing.T) {
	code := `
function Test() {
	this.value = 42;
}
Test.prototype.getValue = function() {
	return this.value;
};

let t = new Test();

// Test each step
console.log("t.value:", t.value);
console.log("Test.prototype.getValue:", Test.prototype.getValue);
console.log("t.getValue:", t.getValue);
// Does t.getValue === Test.prototype.getValue?
t.getValue === Test.prototype.getValue;`

	p := driver.NewPaserati()
	result, errs := p.RunString(code)
	if len(errs) > 0 {
		t.Fatalf("Test failed: %v", errs)
	}
	t.Logf("Method equality: %s = %v", result.TypeName(), result.Inspect())
}