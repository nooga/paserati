package vm

import "github.com/nooga/paserati/pkg/runtime"

// SetAsyncRuntime sets the async execution runtime
func (vm *VM) SetAsyncRuntime(rt runtime.AsyncRuntime) {
	vm.asyncRuntime = rt
}

// GetAsyncRuntime returns the current async runtime (or default)
func (vm *VM) GetAsyncRuntime() runtime.AsyncRuntime {
	if vm.asyncRuntime == nil {
		vm.asyncRuntime = runtime.NewDefaultAsyncRuntime()
	}
	return vm.asyncRuntime
}

// DrainMicrotasks runs all pending microtasks until idle
func (vm *VM) DrainMicrotasks() {
	rt := vm.GetAsyncRuntime()
	iterations := 0
	for rt.RunUntilIdle() {
		iterations++
		if iterations > 1000 {
			break // Safety: prevent infinite microtask loops
		}
	}
}

// DrainUntilIdle runs the full host event-loop drain: nextTicks, microtasks,
// due timers, macrotasks, then waits for external ops or future timers.
func (vm *VM) DrainUntilIdle() {
	rt := vm.GetAsyncRuntime()
	iterations := 0
	for {
		if rt.RunNextTicks() {
			iterations++
			continue
		}
		if rt.RunUntilIdle() {
			iterations++
			continue
		}
		if rt.RunDueTimers() {
			iterations++
			continue
		}
		if rt.RunMacrotasks() {
			iterations++
			continue
		}
		if rt.HasPendingExternalOps() {
			rt.WaitForExternalOp()
			iterations++
			continue
		}
		if rt.HasPendingTimers() {
			rt.WaitForIdleProgress()
			iterations++
			continue
		}
		if !rt.HasPendingWork() {
			return
		}
		iterations++
		if iterations > 1_000_000 {
			return
		}
	}
}
