package driver

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/vm"
)

func newHostTimerPaserati() *Paserati {
	inits := append(builtins.GetStandardInitializers(), NewHostTimerInitializer())
	p := NewPaseratiWithInitializers(inits)
	p.SetSkipTypeCheck(true)
	return p
}

func hostArrayJoin(t *testing.T, p *Paserati, name string) string {
	t.Helper()
	val, ok := p.GetVM().GetGlobal(name)
	if !ok {
		t.Fatalf("global %q not found", name)
	}
	if !val.IsArray() {
		t.Fatalf("global %q is not an array", name)
	}
	arr := val.AsArray()
	parts := make([]string, arr.Length())
	for i := 0; i < arr.Length(); i++ {
		parts[i] = arr.Get(i).ToString()
	}
	return strings.Join(parts, ",")
}

func hostGlobalTruthy(t *testing.T, p *Paserati, name string) vm.Value {
	t.Helper()
	val, ok := p.GetVM().GetGlobal(name)
	if !ok {
		t.Fatalf("global %q not found", name)
	}
	return val
}

func TestHostTimersNotInStandardBuiltins(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)
	_, errs := p.RunCode(`typeof setTimeout`, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	val, ok := p.GetVM().GetGlobal("setTimeout")
	if ok && val.IsCallable() {
		t.Fatal("setTimeout must not be a standard builtin; use NewHostTimerInitializer")
	}
}

func TestHostNextTickBeforeMicrotask(t *testing.T) {
	p := newHostTimerPaserati()

	js := `
		let order = []
		nextTick(() => order.push("tick"))
		Promise.resolve().then(() => order.push("micro"))
		order.join(",")
	`
	_, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	if got := hostArrayJoin(t, p, "order"); got != "tick,micro" {
		t.Errorf("expected tick,micro, got %q", got)
	}
}

func TestHostMicrotaskBeforeTimeoutZero(t *testing.T) {
	p := newHostTimerPaserati()

	js := `
		let order = []
		Promise.resolve().then(() => order.push("micro"))
		setTimeout(() => order.push("timer"), 0)
		order.join(",")
	`
	_, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	if got := hostArrayJoin(t, p, "order"); got != "micro,timer" {
		t.Errorf("expected micro,timer, got %q", got)
	}
}

func TestHostDrainWaitsForTimer(t *testing.T) {
	p := newHostTimerPaserati()

	js := `
		let done = false
		setTimeout(() => { done = true }, 40)
		done
	`
	_, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	done := hostGlobalTruthy(t, p, "done")
	if !done.IsTruthy() {
		t.Errorf("expected done=true after drain, got %v", done.ToString())
	}
}

func TestHostClearTimeout(t *testing.T) {
	p := newHostTimerPaserati()

	js := `
		let fired = false
		let id = setTimeout(() => { fired = true }, 30)
		clearTimeout(id)
		fired
	`
	_, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	// Last-statement `fired` is captured before drain; the invariant is that
	// DrainUntilIdle must not run the cancelled timer.
	fired := hostGlobalTruthy(t, p, "fired")
	if fired.IsTruthy() {
		t.Errorf("expected fired=false after drain, got %v", fired.ToString())
	}
}

func TestHostTLAWaitsForTimer(t *testing.T) {
	p := newHostTimerPaserati()

	js := `
		await new Promise((resolve) => setTimeout(resolve, 40));
		true;
	`
	result, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("TLA+timer failed: %v", errs[0])
	}
	if result.ToString() != "true" {
		t.Errorf("expected true after awaited timer, got %v", result.ToString())
	}
}

// captureStderr redirects os.Stderr to a pipe for the duration of the test and
// returns a function that closes the pipe and yields everything written to
// it. Needed because reportUncaughtTimerException writes straight to
// os.Stderr rather than through an injectable writer.
func captureStderr(t *testing.T) func() string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = old })
	return func() string {
		w.Close()
		os.Stderr = old
		var buf bytes.Buffer
		io.Copy(&buf, r)
		return buf.String()
	}
}

// captureExit swaps in a fake osExit that records the requested code instead
// of terminating the test binary, and restores the real one afterwards.
func captureExit(t *testing.T) *bool {
	t.Helper()
	exited := false
	old := osExit
	osExit = func(code int) {
		exited = true
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
	}
	t.Cleanup(func() { osExit = old })
	return &exited
}

// #484: an exception thrown inside a setTimeout callback has no catching JS
// context above it (unlike a normal call stack), so it must be reported and
// terminate the process the way a genuinely uncaught top-level exception
// does - not silently discarded, leaving the process to exit 0 as if the
// throw never happened.
func TestHostSetTimeoutUncaughtExceptionReportsAndExits(t *testing.T) {
	exited := captureExit(t)
	stderr := captureStderr(t)
	p := newHostTimerPaserati()

	js := `
		setTimeout(() => { throw new Error("boom from timeout") }, 0)
	`
	_, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	if !*exited {
		t.Fatal("expected the uncaught exception to trigger a process exit")
	}
	if got := stderr(); !strings.Contains(got, "Uncaught exception: Error: boom from timeout") {
		t.Errorf("expected stderr to report the uncaught exception, got %q", got)
	}
}

// Same as above but for nextTick, which shares the exact discard bug.
func TestHostNextTickUncaughtExceptionReportsAndExits(t *testing.T) {
	exited := captureExit(t)
	stderr := captureStderr(t)
	p := newHostTimerPaserati()

	js := `
		nextTick(() => { throw new Error("boom from nextTick") })
	`
	_, errs := p.RunCode(js, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	if !*exited {
		t.Fatal("expected the uncaught exception to trigger a process exit")
	}
	if got := stderr(); !strings.Contains(got, "Uncaught exception: Error: boom from nextTick") {
		t.Errorf("expected stderr to report the uncaught exception, got %q", got)
	}
}
