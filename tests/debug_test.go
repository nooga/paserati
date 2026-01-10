package tests

import (
	"testing"

	"github.com/nooga/paserati/pkg/driver"
)

func TestDebugFunctionCall(t *testing.T) {
	code := `
function multiply(x: number, y: number) {
	return x * y;
}

multiply.call(null, 6, 7);`

	p := driver.NewPaserati()
	result, errs := p.RunString(code)
	if len(errs) > 0 {
		t.Fatalf("Evaluation failed: %v", errs)
	}

	t.Logf("Result type: %s", result.TypeName())
	t.Logf("Result value: %v", result.Inspect())
}

func TestDebugFunctionAccess(t *testing.T) {
	code := `
function MyFunc() {
	this.value = 42;
}
MyFunc.prototype.getValue = function() {
	return this.value;
};

let instance = new MyFunc();
instance.getValue();`

	p := driver.NewPaserati()
	result, errs := p.RunString(code)
	if len(errs) > 0 {
		t.Fatalf("Evaluation failed: %v", errs)
	}

	t.Logf("Result type: %s", result.TypeName())
	t.Logf("Result value: %v", result.Inspect())
}
