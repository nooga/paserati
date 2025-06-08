package tests

import (
	"testing"
	"paserati/pkg/driver"
)

func TestWorkingPrototype(t *testing.T) {
	// Test the exact same pattern as prototype_methods.ts
	code := `
function Counter(initial) {
  this.count = initial;
}

Counter.prototype.increment = function () {
  this.count++;
  return this.count;
};

let c1 = new Counter(10);
c1.increment();`

	p := driver.NewPaserati()
	result, errs := p.RunString(code)
	if len(errs) > 0 {
		t.Fatalf("Test failed: %v", errs)
	}
	t.Logf("Working prototype call: %s = %v", result.TypeName(), result.Inspect())
}

func TestMyPrototype(t *testing.T) {
	// Test my failing pattern
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
		t.Fatalf("Test failed: %v", errs)
	}
	t.Logf("My prototype call: %s = %v", result.TypeName(), result.Inspect())
}