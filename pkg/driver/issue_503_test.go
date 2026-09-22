package driver

import (
	"testing"
	"time"

	"github.com/nooga/paserati/pkg/vm"
)

// TestReentrantRunScriptDoesNotDeadlockOnPendingExternalOp covers paserati#503:
// runAsScript/runAsModule used to unconditionally call DrainUntilIdle() after
// Interpret(), even for a reentrant RunScript call made from inside a native
// function invoked by already-executing script code (e.g. a host's
// synchronous require() compiling and running a not-yet-loaded module).
//
// DrainUntilIdle's WaitForExternalOp branch blocks until *some* external op
// completes - fine for the true top-level call (a long-lived server's own
// RunScript is supposed to keep draining/blocking for as long as it keeps an
// external op open, e.g. http.Server.listen() - that part is correct,
// intended behavior and this test does not wait on it). But a *nested* call
// has no business waiting on an unrelated external op that the host
// intentionally keeps open for the whole process lifetime. That made every
// first-time synchronous require() done while such an op was pending hang
// forever, even though the nested script itself had nothing left to do.
func TestReentrantRunScriptDoesNotDeadlockOnPendingExternalOp(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	vmInst := p.GetVM()
	rt := vmInst.GetAsyncRuntime()

	// Simulate a long-lived server holding a permanent external op open,
	// exactly like http.Server.listen() would for the process lifetime.
	rt.BeginExternalOp()
	defer rt.EndExternalOp()

	nestedDone := make(chan struct{})
	global, _ := vmInst.GetGlobal("globalThis")
	obj := global.AsPlainObject()
	obj.SetOwn("nestedRunScript", vm.NewNativeFunction(0, false, "nestedRunScript", func(_ []vm.Value) (vm.Value, error) {
		defer close(nestedDone)
		v, errs := p.RunScript(`1 + 1`, "nested.js")
		if len(errs) > 0 {
			t.Errorf("nested RunScript returned errors: %v", errs)
		}
		if !v.IsNumber() || v.ToFloat() != 2 {
			t.Errorf("nested RunScript returned %s, want 2", v.Inspect())
		}
		return vm.Undefined, nil
	}))

	// The top-level RunScript is the one legitimately expected to keep
	// draining/blocking for as long as the external op stays open (that's
	// how a real host's server-starting script is meant to behave), so it
	// runs in the background and this test never waits on it - only on the
	// nested call completing promptly.
	go func() {
		p.RunScript(`globalThis.nestedRunScript(); "top-level done"`, "top.js")
	}()

	select {
	case <-nestedDone:
		// PASS: nested RunScript returned promptly instead of blocking on
		// the still-pending external op.
	case <-time.After(5 * time.Second):
		t.Fatal("nested RunScript call hung for 5s (deadlocked in DrainUntilIdle)")
	}
}
