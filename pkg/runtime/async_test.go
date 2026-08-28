package runtime

import (
	"sync/atomic"
	"testing"
	"time"
)

func drainUntilIdle(rt AsyncRuntime) {
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

func TestNextTickBeforeMicrotasks(t *testing.T) {
	rt := NewDefaultAsyncRuntime()
	var order []string

	rt.ScheduleMicrotask(func() {
		order = append(order, "micro")
	})
	rt.ScheduleNextTick(func() {
		order = append(order, "nextTick")
	})

	drainUntilIdle(rt)

	if len(order) != 2 || order[0] != "nextTick" || order[1] != "micro" {
		t.Fatalf("expected [nextTick micro], got %v", order)
	}
}

func TestMicrotasksBeforeTimerZero(t *testing.T) {
	rt := NewDefaultAsyncRuntime()
	var order []string

	rt.ScheduleTimer(0, func() {
		order = append(order, "timer")
	})
	rt.ScheduleMicrotask(func() {
		order = append(order, "micro")
	})

	drainUntilIdle(rt)

	if len(order) != 2 || order[0] != "micro" || order[1] != "timer" {
		t.Fatalf("expected [micro timer], got %v", order)
	}
}

func TestScheduleTimerWaitsForDelay(t *testing.T) {
	rt := NewDefaultAsyncRuntime()
	var ran atomic.Bool

	rt.ScheduleTimer(30*time.Millisecond, func() {
		ran.Store(true)
	})

	start := time.Now()
	drainUntilIdle(rt)
	elapsed := time.Since(start)

	if !ran.Load() {
		t.Fatal("timer callback did not run")
	}
	if elapsed < 25*time.Millisecond {
		t.Fatalf("expected to wait ~30ms, only waited %v", elapsed)
	}
}

func TestCancelTimerPreventsCallback(t *testing.T) {
	rt := NewDefaultAsyncRuntime()
	var ran atomic.Bool

	id := rt.ScheduleTimer(10*time.Millisecond, func() {
		ran.Store(true)
	})
	rt.CancelTimer(id)

	drainUntilIdle(rt)

	if ran.Load() {
		t.Fatal("cancelled timer callback should not run")
	}
}

func TestResetClearsPendingTimer(t *testing.T) {
	rt := NewDefaultAsyncRuntime()
	var ran atomic.Bool

	rt.ScheduleTimer(50*time.Millisecond, func() {
		ran.Store(true)
	})
	rt.Reset()

	done := make(chan struct{})
	go func() {
		rt.WaitForIdleProgress()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("WaitForIdleProgress blocked after Reset with no pending work")
	}

	if ran.Load() {
		t.Fatal("reset timer callback should not run")
	}
	if rt.HasPendingTimers() {
		t.Fatal("expected no pending timers after Reset")
	}
}
