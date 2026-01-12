package runtime

import "sync"

// AsyncRuntime provides the async execution environment for Paserati
// This interface allows plugging in different async execution strategies
// (Go-based, event loop, deterministic testing, etc.)
type AsyncRuntime interface {
	// ScheduleMicrotask queues a callback to run after current task completes
	// Microtasks run before the next task and are used for Promise resolution
	ScheduleMicrotask(callback func())

	// RunUntilIdle executes all pending microtasks and returns
	// Returns true if any work was done
	RunUntilIdle() bool

	// Reset clears all pending tasks (useful for testing)
	Reset()

	// BeginExternalOp marks the start of an external async operation (HTTP, timers, etc.)
	// This allows the runtime to wait for external operations to complete
	BeginExternalOp()

	// EndExternalOp marks the completion of an external async operation
	// This should be called when the operation completes and resolves/rejects a promise
	EndExternalOp()

	// HasPendingExternalOps returns true if there are pending external operations
	HasPendingExternalOps() bool

	// WaitForExternalOp blocks until at least one external operation completes
	// Returns immediately if there are no pending external operations
	WaitForExternalOp()
}

// DefaultAsyncRuntime is a simple Go-based runtime with a microtask queue
type DefaultAsyncRuntime struct {
	microtasks      []func()
	mu              sync.Mutex
	pendingExternal int
	externalCond    *sync.Cond
}

// NewDefaultAsyncRuntime creates a new default async runtime
func NewDefaultAsyncRuntime() *DefaultAsyncRuntime {
	rt := &DefaultAsyncRuntime{
		microtasks: make([]func(), 0, 16),
	}
	rt.externalCond = sync.NewCond(&rt.mu)
	return rt
}

// ScheduleMicrotask adds a callback to the microtask queue
func (rt *DefaultAsyncRuntime) ScheduleMicrotask(callback func()) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.microtasks = append(rt.microtasks, callback)
}

// RunUntilIdle executes all pending microtasks
// Returns true if any microtasks were executed
func (rt *DefaultAsyncRuntime) RunUntilIdle() bool {
	rt.mu.Lock()
	tasks := rt.microtasks
	rt.microtasks = make([]func(), 0, 16)
	rt.mu.Unlock()

	if len(tasks) == 0 {
		return false
	}

	// Execute all microtasks
	// Note: New microtasks scheduled during execution will be processed
	// in the next call to RunUntilIdle (matching JavaScript semantics)
	for _, task := range tasks {
		task()
	}

	return true
}

// Reset clears all pending microtasks
func (rt *DefaultAsyncRuntime) Reset() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.microtasks = make([]func(), 0, 16)
	rt.pendingExternal = 0
}

// BeginExternalOp marks the start of an external async operation
func (rt *DefaultAsyncRuntime) BeginExternalOp() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.pendingExternal++
}

// EndExternalOp marks the completion of an external async operation
func (rt *DefaultAsyncRuntime) EndExternalOp() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.pendingExternal--
	// Signal any waiters that an operation completed
	rt.externalCond.Broadcast()
}

// HasPendingExternalOps returns true if there are pending external operations
func (rt *DefaultAsyncRuntime) HasPendingExternalOps() bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.pendingExternal > 0
}

// WaitForExternalOp blocks until at least one external operation completes
func (rt *DefaultAsyncRuntime) WaitForExternalOp() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.pendingExternal > 0 {
		rt.externalCond.Wait()
	}
}
