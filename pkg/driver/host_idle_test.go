package driver

import (
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
