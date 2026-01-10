package tests

import (
	"testing"

	"github.com/nooga/paserati/pkg/driver"
)

func TestDirectMethodCall(t *testing.T) {
	// Let's test the exact issue step by step
	code1 := `
function Test() {
	this.value = 42;
}
Test.prototype.getValue = function() {
	return this.value;
};

let t = new Test();
t;` // Just the object

	p1 := driver.NewPaserati()
	result1, errs1 := p1.RunString(code1)
	if len(errs1) > 0 {
		t.Fatalf("Step 1 failed: %v", errs1)
	}
	t.Logf("Step 1 - object: %s = %v", result1.TypeName(), result1.Inspect())

	code2 := `
function Test() {
	this.value = 42;
}
Test.prototype.getValue = function() {
	return this.value;
};

let t = new Test();
t.getValue;` // Just the method

	p2 := driver.NewPaserati()
	result2, errs2 := p2.RunString(code2)
	if len(errs2) > 0 {
		t.Fatalf("Step 2 failed: %v", errs2)
	}
	t.Logf("Step 2 - method: %s = %v", result2.TypeName(), result2.Inspect())

	code3 := `
function Test() {
	this.value = 42;
}
Test.prototype.getValue = function() {
	return this.value;
};

let t = new Test();
t.getValue();` // Method call

	p3 := driver.NewPaserati()
	result3, errs3 := p3.RunString(code3)
	if len(errs3) > 0 {
		t.Fatalf("Step 3 failed: %v", errs3)
	}
	t.Logf("Step 3 - method call: %s = %v", result3.TypeName(), result3.Inspect())

	// Test with a method that returns a simple value
	code4 := `
function Test() {
	this.value = 42;
}
Test.prototype.getNumber = function() {
	return 100;
};

let t = new Test();
t.getNumber();` // Method call with simple return

	p4 := driver.NewPaserati()
	result4, errs4 := p4.RunString(code4)
	if len(errs4) > 0 {
		t.Fatalf("Step 4 failed: %v", errs4)
	}
	t.Logf("Step 4 - simple method call: %s = %v", result4.TypeName(), result4.Inspect())
}
