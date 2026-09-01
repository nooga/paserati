package vm

import "testing"

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
