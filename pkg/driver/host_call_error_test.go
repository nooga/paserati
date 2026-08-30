package driver

import (
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

// TestHostCallSurfacesInternalDiagnostic verifies #130: when a JS function
// invoked directly by an embedder via vm.Call() fails through
// vm.runtimeError() (an internal-invariant failure with no JS exception
// value to hand back - e.g. a stack overflow mid-construction, a corrupted
// register/constant index, a recovered Go panic) rather than a real thrown
// exception, executeUserFunctionSafe must surface the VM's own recorded
// diagnostic instead of discarding it for a fixed, uninformative string.
func TestHostCallSurfacesInternalDiagnostic(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	js := `
class Foo {
  constructor(n) {
    new Foo(n + 1);
  }
}
function trigger() {
  new Foo(0);
}
`
	_, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}

	vmInst := p.GetVM()
	fn, ok := vmInst.GetGlobal("trigger")
	if !ok {
		t.Fatal("global 'trigger' not found")
	}

	result, err := vmInst.Call(fn, vm.Undefined, nil)
	t.Logf("result=%v err=%v", result, err)
	if err == nil {
		t.Fatal("expected an error from vm.Call, got nil")
	}
	if err.Error() == "runtime error during user function execution" {
		t.Errorf("vm.Call lost the real diagnostic and returned the generic fallback string: %v", err)
	}
	if !strings.Contains(err.Error(), "Stack overflow during constructor call") {
		t.Errorf("expected the real diagnostic to survive, got: %v", err)
	}
}

// TestHostRunCodeStillReportsInternalDiagnosticCleanly guards against a
// regression found while fixing #130: an earlier version of the fix had
// executeUserFunctionSafe *consume* (pop) the diagnostic from vm.errors when
// surfacing it as a Go error. That's correct only when executeUserFunctionSafe
// is the terminal consumer (as it is for a direct embedder vm.Call(), see
// above). Here the same runtimeError() fires inside a callback invoked from
// bytecode that is itself running under RunCode's own top-level Interpret(),
// which reports vm.errors wholesale once the (re-wrapped, re-thrown)
// exception finishes propagating. Popping the entry left that top-level
// report with nothing to show but the newly-thrown wrapper exception's own
// multi-thousand-frame stack trace (from constructing `new Error(...)` while
// the VM's frame stack was still at the depth the overflow itself left it at)
// instead of the one-line diagnostic RunCode reports today.
func TestHostRunCodeStillReportsInternalDiagnosticCleanly(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	js := `
class Foo {
  constructor(n) {
    new Foo(n + 1);
  }
}
[1].forEach(() => { new Foo(0); });
`
	_, errs := p.RunCode(js, RunOptions{})
	if len(errs) == 0 {
		t.Fatal("expected RunCode to report an error")
	}
	joined := errs[0].Error()
	t.Logf("error: %s", joined)
	if len(joined) > 2000 {
		t.Errorf("error message is suspiciously long (%d chars) - looks like the garbage-stack-trace regression, not a clean diagnostic", len(joined))
	}
	if !strings.Contains(joined, "Stack overflow during constructor call") {
		t.Errorf("expected the real diagnostic in the reported error, got: %s", joined)
	}
}
