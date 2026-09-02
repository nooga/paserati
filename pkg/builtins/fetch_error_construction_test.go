package builtins

import (
	"sync"
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

// TestFetchErrorConstructionIsGoroutineSafe is the regression test for the
// vm.Call-from-background-goroutine hazard flagged after #212/#213/#214:
// vmInstance.NewTypeError(...) builds its Error instance by calling the
// TypeError constructor via vm.Call, which mutates the VM's shared
// currentThis/currentNewTarget fields with no synchronization against the
// main interpreter loop - unsafe from any goroutine but the one driving the
// VM. fetch()'s two body-JSON-serialization error paths run inside
// doFetchRequestWithContext, which executes entirely on its own background
// goroutine (see fetch_init.go), so they can't safely use
// vmInstance.NewTypeError. newTypeError/newTypeErrorValue/
// newErrorValueWithPrototype build the instance by hand instead, touching
// only data freshly allocated for the call.
//
// This drives that exact mechanism concurrently: one goroutine repeatedly
// calls a native function through vm.Call (mutating currentThis/
// currentNewTarget, mirroring the main interpreter loop's own native calls)
// while several others repeatedly build TypeError/AbortError values via the
// new helpers. Run with -race; a regression back to vm.Call-based
// construction here reliably trips the race detector.
func TestFetchErrorConstructionIsGoroutineSafe(t *testing.T) {
	vmInstance := vm.NewVM()

	// A trivial native function whose call convention is exactly what
	// vm.Call exercises for Error/TypeError: entering via
	// enterOrdinaryNativeCall, which sets currentThis (see vm.Call's
	// TypeNativeFunctionWithProps case).
	fn := vm.NewNativeFunction(0, false, "noop", func(args []vm.Value) (vm.Value, error) {
		return vm.Undefined, nil
	})

	const iterations = 2000
	var wg sync.WaitGroup

	// "Main" goroutine: repeatedly calls into the VM the way the
	// interpreter loop does for any native call.
	wg.Go(func() {
		for range iterations {
			if _, err := vmInstance.Call(fn, vm.Undefined, nil); err != nil {
				t.Errorf("vm.Call(noop) failed: %v", err)
				return
			}
		}
	})

	// Several background goroutines building fetch()'s error values
	// concurrently - the exact shape of doFetchRequestWithContext's own
	// goroutine racing the interpreter loop.
	for range 4 {
		wg.Go(func() {
			for range iterations {
				err := newTypeError(vmInstance, "failed to serialize body to JSON: boom")
				exc, ok := err.(vm.ExceptionError)
				if !ok {
					t.Errorf("newTypeError did not return a vm.ExceptionError: %T", err)
					return
				}
				val := exc.GetExceptionValue()
				if val.Type() != vm.TypeObject {
					t.Errorf("exception value is not an object: %v", val.Type())
					return
				}
				obj := val.AsPlainObject()
				if nameVal, _ := obj.GetOwn("name"); nameVal.ToString() != "TypeError" {
					t.Errorf("name = %q, want %q", nameVal.ToString(), "TypeError")
					return
				}
				if obj.GetPrototype() != vmInstance.TypeErrorPrototype {
					t.Errorf("prototype is not vmInstance.TypeErrorPrototype")
					return
				}

				_ = newAbortErrorValue(vmInstance, "aborted")
			}
		})
	}

	wg.Wait()
}
