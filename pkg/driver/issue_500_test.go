package driver

import (
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

// TestVMSetPropertyOnFunctionWritesOwnProperty covers paserati#500: the
// Go-callable vm.Value.SetProperty used to silently no-op for a function
// value (TypeFunction/TypeClosure fell to SetProperty's `default: return
// nil` case), even though ordinary bytecode `Foo.bar = 42` writes into the
// same fn.Properties/cl.Properties table just fine.
func TestVMSetPropertyOnFunctionWritesOwnProperty(t *testing.T) {
	p := NewPaserati()
	vmInst := p.GetVM()

	var observed vm.Value
	p.DeclareModule("probe500set", func(m *ModuleBuilder) {
		m.Function("check", func(ctor vm.Value) {
			if err := vmInst.SetProperty(ctor, "extra", vm.NumberValue(1)); err != nil {
				t.Fatalf("SetProperty returned an error: %v", err)
			}
			v, err := vmInst.GetProperty(ctor, "extra")
			if err != nil {
				t.Fatalf("GetProperty returned an error: %v", err)
			}
			observed = v
		})
	})

	_, errs := p.RunString(`
		import { check } from "probe500set";
		function Ctor() {}
		check(Ctor);
	`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !observed.IsNumber() || observed.ToFloat() != 1 {
		t.Fatalf("expected SetProperty's write to be visible via GetProperty as 1, got %s", observed.Inspect())
	}
}

// TestVMGetPropertyOnFunctionLazilyCreatesPrototype covers the other half of
// paserati#500: vm.Value.GetProperty used to answer Undefined for a
// function's "prototype", instead of lazily creating it the way ordinary
// bytecode `Foo.prototype` access (and `typeof Foo.prototype === "object"`)
// already does.
func TestVMGetPropertyOnFunctionLazilyCreatesPrototype(t *testing.T) {
	p := NewPaserati()
	vmInst := p.GetVM()

	var observed vm.Value
	p.DeclareModule("probe500get", func(m *ModuleBuilder) {
		m.Function("check", func(ctor vm.Value) {
			v, err := vmInst.GetProperty(ctor, "prototype")
			if err != nil {
				t.Fatalf("GetProperty returned an error: %v", err)
			}
			observed = v
		})
	})

	_, errs := p.RunString(`
		import { check } from "probe500get";
		function Ctor() {}
		check(Ctor);
	`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if observed.Type() != vm.TypeObject {
		t.Fatalf("expected GetProperty(ctor, \"prototype\") to lazily create a real object, got %s (%s)", observed.Inspect(), observed.TypeName())
	}
}
