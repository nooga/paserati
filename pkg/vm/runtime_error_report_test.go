package vm

import (
	"testing"

	"github.com/nooga/paserati/pkg/errors"
)

// TestRuntimeErrorSurvivesBrokenFrameCount covers the second half of #156.
//
// runtimeError is the *reporting* path: run()'s panic recovery calls it to turn
// a panic into a diagnostic. It therefore has to stay safe when frame
// bookkeeping is already broken, which is exactly when it gets called. Its
// guard used to be `frameCount == 0`, so a negative count fell straight through
// to vm.frames[frameCount-1] and panicked a second time - printing
// "runtimeError itself panicked while reporting the above" and burying the real
// diagnostic under an internal error.
//
// The first half of #156 (what let frameCount go negative in the first place)
// is fixed separately, in the interpreter's post-unwind paths. This test is
// deliberately independent of that: it asserts the reporter is robust, not that
// the condition never happens.
func TestRuntimeErrorSurvivesBrokenFrameCount(t *testing.T) {
	for _, frameCount := range []int{0, -1, -2} {
		vm := NewVM()
		vm.frameCount = frameCount

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("runtimeError panicked with frameCount=%d: %v", frameCount, r)
				}
			}()
			if got := vm.runtimeError("probe %d", frameCount); got != InterpretRuntimeError {
				t.Errorf("frameCount=%d: got status %v, want InterpretRuntimeError", frameCount, got)
			}
		}()

		// The diagnostic must still be recorded, not swallowed.
		if len(vm.errors) != 1 {
			t.Errorf("frameCount=%d: recorded %d errors, want 1", frameCount, len(vm.errors))
			continue
		}
		if msg := vm.errors[0].Message(); msg == "" {
			t.Errorf("frameCount=%d: recorded an empty message", frameCount)
		}
	}
}

// TestRuntimeErrorFrameSyntheticPositionUsesColumnsTable covers #153:
// runtimeError's frame-synthesized position - reached whenever a VM-internal
// invariant check fails (e.g. "Cannot use 'number' as a constructor"; see
// vm.go/op_*.go's many direct vm.runtimeError(...) call sites) rather than a
// JS-catchable throw - used to hardcode Column to 1 unconditionally, because
// Chunk had no column data to recover at all, only a Lines table. It now also
// consults the chunk's sparse Columns table (Chunk.Columns, populated by the
// compiler's markPosition), so a chunk that has an entry for the failing
// instruction's offset gets a real column instead.
//
// This drives runtimeError directly against a hand-built frame/chunk (as
// TestRuntimeErrorSurvivesBrokenFrameCount above does) rather than through
// real script execution, because every reachable-from-JS runtimeError call
// site this repo could find happens to get wrapped into a catchable
// TypeError first (see TestRuntimeErrorFrameSyntheticPositionHasRealColumn in
// pkg/driver for that path, which goes through throwException instead).
func TestRuntimeErrorFrameSyntheticPositionUsesColumnsTable(t *testing.T) {
	chunk := &Chunk{
		Code:    []byte{byte(OpNop), byte(OpNop), byte(OpNop)},
		Lines:   []int{5, 5, 5},
		Columns: []ColumnEntry{{Offset: 0, Column: 9}},
	}
	fn := &FunctionObject{Name: "foo", Chunk: chunk}
	closure := &ClosureObject{Fn: fn}

	vm := NewVM()
	vm.frameCount = 1
	vm.frames[0] = CallFrame{closure: closure, ip: 2} // instructionPos = ip-1 = 1

	if got := vm.runtimeError("boom"); got != InterpretRuntimeError {
		t.Fatalf("got status %v, want InterpretRuntimeError", got)
	}
	if len(vm.errors) != 1 {
		t.Fatalf("recorded %d errors, want 1", len(vm.errors))
	}
	re, ok := vm.errors[0].(*errors.RuntimeError)
	if !ok {
		t.Fatalf("error is %T, want *errors.RuntimeError", vm.errors[0])
	}
	if re.Position.Line != 5 {
		t.Errorf("line = %d, want 5", re.Position.Line)
	}
	if re.Position.Column != 9 {
		t.Errorf("column = %d, want 9 (from chunk.Columns) - got the old hardcoded-to-1 fallback instead", re.Position.Column)
	}
}

// TestRuntimeErrorFrameSyntheticPositionFallsBackWithoutColumnsTable covers
// the other half of #153: a chunk with no Columns data at all (compiled
// before this table existed, or hand-assembled, like
// TestRuntimeErrorSurvivesBrokenFrameCount's zero-value Chunk above) must
// still fall back to column 1 rather than 0 or some other nonsense value.
func TestRuntimeErrorFrameSyntheticPositionFallsBackWithoutColumnsTable(t *testing.T) {
	chunk := &Chunk{
		Code:  []byte{byte(OpNop), byte(OpNop)},
		Lines: []int{7, 7},
		// Columns intentionally left empty.
	}
	fn := &FunctionObject{Name: "foo", Chunk: chunk}
	closure := &ClosureObject{Fn: fn}

	vm := NewVM()
	vm.frameCount = 1
	vm.frames[0] = CallFrame{closure: closure, ip: 1}

	vm.runtimeError("boom")
	re, ok := vm.errors[0].(*errors.RuntimeError)
	if !ok {
		t.Fatalf("error is %T, want *errors.RuntimeError", vm.errors[0])
	}
	if re.Position.Column != 1 {
		t.Errorf("column = %d, want the column-1 fallback", re.Position.Column)
	}
}
