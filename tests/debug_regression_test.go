package tests

import (
	"testing"
	"paserati/pkg/driver"
)

func TestExactPrototypeMethods(t *testing.T) {
	// Test the exact same code as the working prototype_methods.ts
	code := `
function Counter(initial) {
  this.count = initial;
}

Counter.prototype.increment = function () {
  this.count++;
  return this.count;
};

let c1 = new Counter(10);
let result = c1.increment();
result;`

	p := driver.NewPaserati()
	result, errs := p.RunString(code)
	if len(errs) > 0 {
		t.Fatalf("Test failed: %v", errs)
	}
	t.Logf("Counter increment result: %s = %v", result.TypeName(), result.Inspect())
	
	// Test simple function call
	code2 := `
function getValue() {
	return 42;
}
let result = getValue();
result;`

	p2 := driver.NewPaserati()
	result2, errs2 := p2.RunString(code2)
	if len(errs2) > 0 {
		t.Fatalf("Test2 failed: %v", errs2)
	}
	t.Logf("Function call result: %s = %v", result2.TypeName(), result2.Inspect())
}