package tests

import (
	"testing"

	"github.com/nooga/paserati/pkg/driver"
)

func TestSimpleMethod(t *testing.T) {
	code := `
function Test() {
	this.value = 42;
}
Test.prototype.getValue = function() {
	return this.value;
};

let t = new Test();
t.getValue();`

	p := driver.NewPaserati()
	result, errs := p.RunString(code)
	if len(errs) > 0 {
		t.Fatalf("Evaluation failed: %v", errs)
	}

	t.Logf("Result type: %s", result.TypeName())
	t.Logf("Result value: %v", result.Inspect())
}

func TestSimpleCall(t *testing.T) {
	code := `
function add(x, y) {
	return x + y;
}
add(5, 7);`

	p := driver.NewPaserati()
	result, errs := p.RunString(code)
	if len(errs) > 0 {
		t.Fatalf("Evaluation failed: %v", errs)
	}

	t.Logf("Result type: %s", result.TypeName())
	t.Logf("Result value: %v", result.Inspect())
}

func TestFunctionCallMethod(t *testing.T) {
	code := `
function add(x, y) {
	return x + y;
}
add.call;`

	p := driver.NewPaserati()
	result, errs := p.RunString(code)
	if len(errs) > 0 {
		t.Fatalf("Evaluation failed: %v", errs)
	}

	t.Logf("Result type: %s", result.TypeName())
	t.Logf("Result value: %v", result.Inspect())
}
