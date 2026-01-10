package tests

import (
	"testing"

	"github.com/nooga/paserati/pkg/driver"
)

func TestFunctionCallTypes(t *testing.T) {
	// Test direct function call
	code1 := `
function getValue() {
	return 100;
}
getValue();`

	p1 := driver.NewPaserati()
	result1, errs1 := p1.RunString(code1)
	if len(errs1) > 0 {
		t.Fatalf("Direct call failed: %v", errs1)
	}
	t.Logf("Direct call: %s = %v", result1.TypeName(), result1.Inspect())

	// Test method call with 'this'
	code2 := `
let obj = {
	value: 42,
	getValue: function() {
		return this.value;
	}
};
obj.getValue();`

	p2 := driver.NewPaserati()
	result2, errs2 := p2.RunString(code2)
	if len(errs2) > 0 {
		t.Fatalf("Object method call failed: %v", errs2)
	}
	t.Logf("Object method call: %s = %v", result2.TypeName(), result2.Inspect())

	// Test prototype method call
	code3 := `
function Test() {
	this.value = 42;
}
Test.prototype.getValue = function() {
	return this.value;
};
let t = new Test();
let method = t.getValue;
method.call(t);`

	p3 := driver.NewPaserati()
	result3, errs3 := p3.RunString(code3)
	if len(errs3) > 0 {
		t.Fatalf("Manual call failed: %v", errs3)
	}
	t.Logf("Manual call: %s = %v", result3.TypeName(), result3.Inspect())
}
