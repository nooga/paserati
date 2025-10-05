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
}

// DefaultAsyncRuntime is a simple Go-based runtime with a microtask queue
type DefaultAsyncRuntime struct {
	microtasks []func()
	mu         sync.Mutex
}

// NewDefaultAsyncRuntime creates a new default async runtime
func NewDefaultAsyncRuntime() *DefaultAsyncRuntime {
	return &DefaultAsyncRuntime{
		microtasks: make([]func(), 0, 16),
	}
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
}
