package driver

import (
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

// TestNativeFunctionVMValueParameterReceivesRealArgument covers paserati#162:
// a Go native function declared through ModuleBuilder.Function with a
// vm.Value-typed parameter used to always receive a zeroed vm.Value instead
// of the real JS argument, because vmValueToReflectValue had no case for
// reflect.TypeOf(vm.Value{}) itself and fell through to its default
// reflect.Zero(targetType). This made it impossible to accept a raw JS
// callback (or any other value) as a plain vm.Value parameter through the
// declarative ModuleBuilder.Function path - cb.IsCallable() on the zeroed
// value was silently false no matter what was actually passed.
func TestNativeFunctionVMValueParameterReceivesRealArgument(t *testing.T) {
	p := NewPaserati()
	p.DeclareModule("probe162", func(m *ModuleBuilder) {
		m.Function("check", func(cb vm.Value) bool {
			return cb.IsCallable()
		})
	})

	res, errs := p.RunString(`
		import { check } from "probe162";
		check(() => {});
	`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if res != vm.True {
		t.Fatalf("expected the callback argument to reach the native function untouched (IsCallable() true), got %s", res.Inspect())
	}
}
