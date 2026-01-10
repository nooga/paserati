package tests

import (
	"testing"

	"github.com/nooga/paserati/pkg/driver"
)

func TestMethodCallSteps(t *testing.T) {
	// Test each step separately
	code1 := `
function Test() {
	this.value = 42;
}
let t = new Test();
t.value;`

	p1 := driver.NewPaserati()
	result1, errs1 := p1.RunString(code1)
	if len(errs1) > 0 {
		t.Fatalf("Step 1 failed: %v", errs1)
	}
	t.Logf("Step 1 - t.value: %s = %v", result1.TypeName(), result1.Inspect())

	code2 := `
function Test() {
	this.value = 42;
}
Test.prototype.getValue = function() {
	return this.value;
};
let t = new Test();
t.getValue;`

	p2 := driver.NewPaserati()
	result2, errs2 := p2.RunString(code2)
	if len(errs2) > 0 {
		t.Fatalf("Step 2 failed: %v", errs2)
	}
	t.Logf("Step 2 - t.getValue: %s = %v", result2.TypeName(), result2.Inspect())

	code3 := `
function Test() {
	this.value = 42;
}
Test.prototype.getValue = function() {
	return 100; // Simple return
};
let t = new Test();
t.getValue();`

	p3 := driver.NewPaserati()
	result3, errs3 := p3.RunString(code3)
	if len(errs3) > 0 {
		t.Fatalf("Step 3 failed: %v", errs3)
	}
	t.Logf("Step 3 - t.getValue(): %s = %v", result3.TypeName(), result3.Inspect())
}
