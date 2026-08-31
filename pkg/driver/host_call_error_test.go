package driver

import (
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

// newCorruptedConstantFunction builds a minimal, hand-assembled function value
// whose bytecode is deliberately corrupt: its one instruction loads a constant
// at an index past the end of an empty constant pool. Executing it always hits
// vm.runtimeError("Invalid constant index %d", ...) - not a real JS exception,
// just an internal-invariant failure with no exception *value* to report - the
// same shape #130 is about, but deliberately synthetic and stable, so this
// test doesn't need a real, currently-uncaught engine bug as its fixture (an
// earlier version of this test used deep recursive `new Foo()` construction,
// which stopped exercising this code path once #135 made that catchable).
func newCorruptedConstantFunction() vm.Value {
	chunk := &vm.Chunk{
		Code: []byte{
			byte(vm.OpLoadConst), 0, 0xFF, 0xFF, // R0 = Constants[65535] - always out of range
			byte(vm.OpReturnUndefined),
		},
		Constants: nil, // empty pool: any non-negative index is out of range
		Lines:     []int{1, 1, 1, 1, 1},
	}
	return vm.NewFunction(0, 0, 0, 4, false, "corrupted", chunk, false, false, false, false)
}

// TestHostCallSurfacesInternalDiagnostic verifies #130: when a JS function
// invoked directly by an embedder via vm.Call() fails through
// vm.runtimeError() (an internal-invariant failure with no JS exception
// value to hand back) rather than a real thrown exception,
// executeUserFunctionSafe must surface the VM's own recorded diagnostic
// instead of discarding it for a fixed, uninformative string.
func TestHostCallSurfacesInternalDiagnostic(t *testing.T) {
	p := NewPaserati()
	vmInst := p.GetVM()

	result, err := vmInst.Call(newCorruptedConstantFunction(), vm.Undefined, nil)
	t.Logf("result=%v err=%v", result, err)
	if err == nil {
		t.Fatal("expected an error from vm.Call, got nil")
	}
	if err.Error() == "runtime error during user function execution" {
		t.Errorf("vm.Call lost the real diagnostic and returned the generic fallback string: %v", err)
	}
	if !strings.Contains(err.Error(), "Invalid constant index") {
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

	// Inject the corrupted function as a global directly on GlobalObject -
	// OpGetGlobal falls back to it by name when the heap slot for an
	// otherwise-unresolved identifier doesn't exist (the same fallback that
	// makes `Object.defineProperty(this, ...)`-style globals visible).
	vmInst := p.GetVM()
	vmInst.GlobalObject.SetOwn("__trigger", newCorruptedConstantFunction())

	_, errs := p.RunCode(`[1].forEach(() => { __trigger(); });`, RunOptions{})
	if len(errs) == 0 {
		t.Fatal("expected RunCode to report an error")
	}
	joined := errs[0].Error()
	t.Logf("error: %s", joined)
	if len(joined) > 2000 {
		t.Errorf("error message is suspiciously long (%d chars) - looks like the garbage-stack-trace regression, not a clean diagnostic", len(joined))
	}
	if !strings.Contains(joined, "Invalid constant index") {
		t.Errorf("expected the real diagnostic in the reported error, got: %s", joined)
	}
}
