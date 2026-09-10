package vm

import (
	"sync"

	"github.com/nooga/paserati/pkg/runtime"
)

// asyncRuntimeMu guards the lazy initialization in GetAsyncRuntime.
//
// GetAsyncRuntime is reached from host goroutines - anything that settles a
// Promise off the VM's own goroutine ends up scheduling a microtask through it
// (fetch's HTTP goroutine, ReadableStream's pump, and now ResolvePromise's
// thenable branch). Its "create one if nil" write raced against the VM
// goroutine's own first call, which -race reports as a plain unsynchronized
// read/write on vm.asyncRuntime.
var asyncRuntimeMu sync.Mutex

// SetAsyncRuntime sets the async execution runtime
func (vm *VM) SetAsyncRuntime(rt runtime.AsyncRuntime) {
	asyncRuntimeMu.Lock()
	defer asyncRuntimeMu.Unlock()
	vm.asyncRuntime = rt
}

// GetAsyncRuntime returns the current async runtime (or default)
func (vm *VM) GetAsyncRuntime() runtime.AsyncRuntime {
	asyncRuntimeMu.Lock()
	defer asyncRuntimeMu.Unlock()
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
