package driver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestTopLevelAwaitFetchTextDoesNotFalselyDeadlock is a regression test for
// #238: `for (...) { await one(); }` at the top level, where `one()` does
// `const res = await fetch(url); await res.text();`, used to falsely throw
// "Top-level await: promise remains pending with no microtasks to process"
// roughly one iteration in ~15, even though nothing was actually deadlocked.
//
// fetch() and Response.text() each track their own external async
// operation (rt.BeginExternalOp/EndExternalOp) so the driver's event loop
// knows to keep waiting for them. The top-level-await drain loop
// (pkg/vm/vm.go, OpAwait) polls several independent signals in sequence -
// RunNextTicks, RunUntilIdle (microtasks), RunDueTimers, RunMacrotasks,
// HasPendingExternalOps - each under its *own* separate lock/unlock cycle
// rather than one atomic snapshot. That leaves a gap: a background
// goroutine (fetch()'s body-drain goroutine, or Response.text()'s own
// drainBody goroutine) can schedule the microtask that resumes this very
// await *and* end its external op in the window between this loop's
// RunUntilIdle() check (too early - the microtask isn't scheduled yet) and
// its HasPendingExternalOps() check (too late - the op has already ended).
// Every individual check then reports nothing pending, even though a
// microtask is sitting right there in the queue, and the loop wrongly
// declares deadlock.
//
// This runs enough iterations against a real (loopback) HTTP server for
// that race to reproduce reliably before the fix (a `HasPendingWork()`
// atomic-snapshot fallback before giving up, mirroring the identical guard
// already present in VM.DrainUntilIdle).
func TestTopLevelAwaitFetchTextDoesNotFalselyDeadlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	script := `
		async function one() {
			const res = await fetch("` + server.URL + `");
			await res.text();
		}
		for (let i = 0; i < 300; i++) {
			await one();
		}
		"done";
	`

	result, errs := p.RunCode(script, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("script failed (a \"Top-level await\" message here is the #238 false deadlock): %v", errs[0])
	}
	if result.ToString() != "done" {
		t.Fatalf("expected %q, got %v", "done", result.Inspect())
	}
}
