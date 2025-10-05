package vm

import "paserati/pkg/runtime"

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
