package runtime

import (
	"sync"
	"time"
)

const maxNextTicksPerRun = 10000

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

	// ScheduleNextTick queues a process.nextTick-style callback.
	ScheduleNextTick(callback func())

	// RunNextTicks snapshots and runs the current nextTick queue.
	// Returns true if any callbacks ran.
	RunNextTicks() bool

	// ScheduleMacrotask queues a setImmediate-style callback.
	ScheduleMacrotask(callback func())

	// RunMacrotasks snapshots and runs the current macrotask queue.
	// Returns true if any callbacks ran.
	RunMacrotasks() bool

	// ScheduleTimer schedules a timer callback after delay. delay < 0 is treated as 0.
	// Timer expiry goroutines only mark the timer due; callbacks run on the drain thread.
	ScheduleTimer(delay time.Duration, callback func()) (id uint64)

	// CancelTimer cancels a scheduled timer before it fires.
	CancelTimer(id uint64)

	// RunDueTimers runs callbacks for timers that have expired.
	// Returns true if any callbacks ran.
	RunDueTimers() bool

	// HasPendingTimers returns true if timers are scheduled or due but not yet run.
	HasPendingTimers() bool

	// HasPendingWork returns true if any async work remains (nextTicks, microtasks,
	// macrotasks, timers, or external ops).
	HasPendingWork() bool

	// WaitForIdleProgress blocks until a timer is due, an external op completes, or
	// new work is queued. Returns immediately if runnable work is already pending.
	WaitForIdleProgress()
}

type timerEntry struct {
	id       uint64
	deadline time.Time
	callback func()
}

// DefaultAsyncRuntime is a simple Go-based runtime with a microtask queue
type DefaultAsyncRuntime struct {
	microtasks      []func()
	nextTicks       []func()
	macrotasks      []func()
	dueTimers       []*timerEntry
	timers          map[uint64]*timerEntry
	nextTimerID     uint64
	mu              sync.Mutex
	pendingExternal int
	idleCond        *sync.Cond
	wakeWaiters     []chan struct{}
}

// NewDefaultAsyncRuntime creates a new default async runtime
func NewDefaultAsyncRuntime() *DefaultAsyncRuntime {
	rt := &DefaultAsyncRuntime{
		microtasks:  make([]func(), 0, 16),
		nextTicks:   make([]func(), 0, 16),
		macrotasks:  make([]func(), 0, 16),
		dueTimers:   make([]*timerEntry, 0, 8),
		timers:      make(map[uint64]*timerEntry),
		wakeWaiters: make([]chan struct{}, 0, 4),
	}
	rt.idleCond = sync.NewCond(&rt.mu)
	return rt
}

// ScheduleMicrotask adds a callback to the microtask queue
func (rt *DefaultAsyncRuntime) ScheduleMicrotask(callback func()) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.microtasks = append(rt.microtasks, callback)
	rt.signalWaitersLocked()
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

// Reset clears all pending async work
func (rt *DefaultAsyncRuntime) Reset() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.microtasks = make([]func(), 0, 16)
	rt.nextTicks = make([]func(), 0, 16)
	rt.macrotasks = make([]func(), 0, 16)
	rt.dueTimers = make([]*timerEntry, 0, 8)
	rt.timers = make(map[uint64]*timerEntry)
	rt.nextTimerID = 0
	rt.pendingExternal = 0
	rt.signalWaitersLocked()
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
	rt.signalWaitersLocked()
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
		rt.idleCond.Wait()
	}
}

// ScheduleNextTick queues a nextTick callback.
func (rt *DefaultAsyncRuntime) ScheduleNextTick(callback func()) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.nextTicks = append(rt.nextTicks, callback)
	rt.signalWaitersLocked()
}

// RunNextTicks snapshots and runs the current nextTick queue.
func (rt *DefaultAsyncRuntime) RunNextTicks() bool {
	rt.mu.Lock()
	callbacks := rt.nextTicks
	rt.nextTicks = make([]func(), 0, 16)
	rt.mu.Unlock()

	if len(callbacks) == 0 {
		return false
	}

	limit := len(callbacks)
	if limit > maxNextTicksPerRun {
		limit = maxNextTicksPerRun
	}
	for i := 0; i < limit; i++ {
		callbacks[i]()
	}
	if len(callbacks) > maxNextTicksPerRun {
		rt.mu.Lock()
		rt.nextTicks = append(rt.nextTicks, callbacks[maxNextTicksPerRun:]...)
		rt.mu.Unlock()
	}
	return true
}

// ScheduleMacrotask queues a setImmediate-style callback.
func (rt *DefaultAsyncRuntime) ScheduleMacrotask(callback func()) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.macrotasks = append(rt.macrotasks, callback)
	rt.signalWaitersLocked()
}

// RunMacrotasks snapshots and runs the current macrotask queue.
func (rt *DefaultAsyncRuntime) RunMacrotasks() bool {
	rt.mu.Lock()
	callbacks := rt.macrotasks
	rt.macrotasks = make([]func(), 0, 16)
	rt.mu.Unlock()

	if len(callbacks) == 0 {
		return false
	}
	for _, cb := range callbacks {
		cb()
	}
	return true
}

// ScheduleTimer schedules a timer callback after delay.
func (rt *DefaultAsyncRuntime) ScheduleTimer(delay time.Duration, callback func()) uint64 {
	if delay < 0 {
		delay = 0
	}

	rt.mu.Lock()
	rt.nextTimerID++
	id := rt.nextTimerID
	deadline := time.Now().Add(delay)
	rt.timers[id] = &timerEntry{
		id:       id,
		deadline: deadline,
		callback: callback,
	}
	rt.signalWaitersLocked()
	rt.mu.Unlock()

	go rt.timerExpiryGoroutine(id, delay)
	return id
}

func (rt *DefaultAsyncRuntime) timerExpiryGoroutine(id uint64, delay time.Duration) {
	if delay > 0 {
		time.Sleep(delay)
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	entry, ok := rt.timers[id]
	if !ok {
		return
	}
	delete(rt.timers, id)
	rt.dueTimers = append(rt.dueTimers, entry)
	rt.signalWaitersLocked()
}

// CancelTimer cancels a scheduled timer before it fires, including timers
// that have already expired but not yet been drained.
func (rt *DefaultAsyncRuntime) CancelTimer(id uint64) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.timers, id)
	if len(rt.dueTimers) > 0 {
		kept := rt.dueTimers[:0]
		for _, e := range rt.dueTimers {
			if e.id != id {
				kept = append(kept, e)
			}
		}
		rt.dueTimers = kept
	}
	rt.signalWaitersLocked()
}

// RunDueTimers runs callbacks for timers that have expired.
func (rt *DefaultAsyncRuntime) RunDueTimers() bool {
	rt.mu.Lock()
	entries := rt.dueTimers
	rt.dueTimers = make([]*timerEntry, 0, 8)
	rt.mu.Unlock()

	if len(entries) == 0 {
		return false
	}
	for _, e := range entries {
		if e != nil && e.callback != nil {
			e.callback()
		}
	}
	return true
}

// HasPendingTimers returns true if timers are scheduled or due but not yet run.
func (rt *DefaultAsyncRuntime) HasPendingTimers() bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return len(rt.timers) > 0 || len(rt.dueTimers) > 0
}

// HasPendingWork returns true if any async work remains.
func (rt *DefaultAsyncRuntime) HasPendingWork() bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.hasPendingWorkLocked()
}

func (rt *DefaultAsyncRuntime) hasPendingWorkLocked() bool {
	return len(rt.nextTicks) > 0 ||
		len(rt.microtasks) > 0 ||
		len(rt.macrotasks) > 0 ||
		len(rt.timers) > 0 ||
		len(rt.dueTimers) > 0 ||
		rt.pendingExternal > 0
}

func (rt *DefaultAsyncRuntime) hasRunnableWorkLocked() bool {
	return len(rt.nextTicks) > 0 ||
		len(rt.microtasks) > 0 ||
		len(rt.macrotasks) > 0 ||
		len(rt.dueTimers) > 0
}

// nextDueLocked returns the earliest active timer deadline and whether one exists.
// delay <= 0 means due immediately.
func (rt *DefaultAsyncRuntime) nextDueLocked() (time.Time, bool) {
	var earliest time.Time
	found := false
	for _, entry := range rt.timers {
		if !found || entry.deadline.Before(earliest) {
			earliest = entry.deadline
			found = true
		}
	}
	return earliest, found
}

func (rt *DefaultAsyncRuntime) promoteDueTimersLocked() bool {
	if len(rt.dueTimers) > 0 {
		return true
	}

	now := time.Now()
	promoted := false
	for id, entry := range rt.timers {
		if !entry.deadline.After(now) {
			delete(rt.timers, id)
			rt.dueTimers = append(rt.dueTimers, entry)
			promoted = true
		}
	}
	return promoted
}

// WaitForIdleProgress blocks until progress can be made.
func (rt *DefaultAsyncRuntime) WaitForIdleProgress() {
	rt.mu.Lock()
	if rt.hasRunnableWorkLocked() || !rt.hasPendingWorkLocked() {
		rt.mu.Unlock()
		return
	}

	if rt.promoteDueTimersLocked() {
		rt.mu.Unlock()
		return
	}

	deadline, hasTimer := rt.nextDueLocked()
	if !hasTimer {
		rt.idleCond.Wait()
		rt.mu.Unlock()
		return
	}

	wait := time.Until(deadline)
	if wait <= 0 {
		rt.promoteDueTimersLocked()
		rt.mu.Unlock()
		return
	}

	wake := make(chan struct{}, 1)
	rt.wakeWaiters = append(rt.wakeWaiters, wake)
	rt.mu.Unlock()

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-timer.C:
		rt.mu.Lock()
		rt.promoteDueTimersLocked()
		rt.mu.Unlock()
	case <-wake:
	}
}

func (rt *DefaultAsyncRuntime) signalWaitersLocked() {
	rt.idleCond.Broadcast()
	for _, ch := range rt.wakeWaiters {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	rt.wakeWaiters = rt.wakeWaiters[:0]
}
