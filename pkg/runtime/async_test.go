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

// TestUnrefTimerAloneDoesNotBlockDrain is the regression test for #374: an
// unref'd timer must not, by itself, keep the drain loop waiting out its
// delay - HasPendingTimers/HasPendingWork must ignore it while it's still
// pending, exactly like Node's timer.unref().
func TestUnrefTimerAloneDoesNotBlockDrain(t *testing.T) {
	rt := NewDefaultAsyncRuntime()
	var ran atomic.Bool

	rt.ScheduleUnrefTimer(2*time.Second, func() {
		ran.Store(true)
	})

	if rt.HasPendingTimers() {
		t.Fatal("a pending unref'd timer should not count as a pending (ref'd) timer")
	}
	if rt.HasPendingWork() {
		t.Fatal("a pending unref'd timer alone should not count as pending work")
	}

	start := time.Now()
	drainUntilIdle(rt)
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Fatalf("drain loop should return immediately with only an unref'd timer pending, took %v", elapsed)
	}
	if ran.Load() {
		t.Fatal("unref'd timer should not have run before the drain loop gave up waiting on it")
	}
}

// TestUnrefTimerStillFiresAlongsideOtherWork is the flip side: an unref'd
// timer must still run normally once it's due, as long as something else
// (here, a pending external op) is keeping the loop alive for it to be
// picked up - unref only means "doesn't justify waiting by itself".
func TestUnrefTimerStillFiresAlongsideOtherWork(t *testing.T) {
	rt := NewDefaultAsyncRuntime()
	var ran atomic.Bool

	rt.ScheduleUnrefTimer(20*time.Millisecond, func() {
		ran.Store(true)
	})

	rt.BeginExternalOp()
	go func() {
		time.Sleep(80 * time.Millisecond)
		rt.EndExternalOp()
	}()

	drainUntilIdle(rt)

	if !ran.Load() {
		t.Fatal("unref'd timer should have fired while another op kept the loop alive")
	}
}

// TestCancelUnrefTimerPreventsCallback confirms CancelTimer works the same
// way for unref'd timers as it does for ordinary ones.
func TestCancelUnrefTimerPreventsCallback(t *testing.T) {
	rt := NewDefaultAsyncRuntime()
	var ran atomic.Bool

	id := rt.ScheduleUnrefTimer(10*time.Millisecond, func() {
		ran.Store(true)
	})
	rt.CancelTimer(id)

	rt.BeginExternalOp()
	go func() {
		time.Sleep(50 * time.Millisecond)
		rt.EndExternalOp()
	}()

	drainUntilIdle(rt)

	if ran.Load() {
		t.Fatal("cancelled unref'd timer callback should not run")
	}
}
