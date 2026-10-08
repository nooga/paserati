package driver

import (
	"errors"
	"strings"
	"testing"
	"time"

	perrors "github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/vm"
)

// An OffThread async function runs on its own goroutine: JS keeps running
// while it works (here JS itself unblocks it - which would deadlock if it
// ran on the VM's thread), and the promise settles from the event loop
// (#623).
func TestAsyncFunctionOffThread(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)
	release := make(chan struct{})
	p.DeclareModule("io", func(m *ModuleBuilder) {
		m.AsyncFunction("slow", func(s string, n float64) (map[string]interface{}, error) {
			<-release
			return map[string]interface{}{"s": strings.ToUpper(s), "n": n * 2}, nil
		}, OffThread)
		m.AsyncFunction("fails", func() error { return errors.New("nope") }, OffThread)
		m.Function("release", func() { close(release) })
	})
	v, errs := p.RunCode(`
		import { slow, fails, release } from "io";
		const order = [];
		const pending = slow("ab", 21).then(r => order.push("slow:" + r.s + r.n));
		order.push("sync");
		release();
		await pending;
		let rejected = "";
		try { await fails(); } catch (e) { rejected = (e instanceof Error) + ":" + e.message; }
		order.join(",") + "|" + rejected
	`, RunOptions{})
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	if got := v.ToString(); got != "sync,slow:AB42|true:nope" {
		t.Fatalf("got %q", got)
	}
}

// Timers keep firing while an external operation is in flight: the event
// loop must not block on the external op alone (#623).
func TestTimersFireDuringExternalOp(t *testing.T) {
	p := newHostTimerPaserati()
	p.SetSkipTypeCheck(true)
	release := make(chan struct{})
	release2 := make(chan struct{})
	p.DeclareModule("io", func(m *ModuleBuilder) {
		m.AsyncFunction("wait", func() { <-release }, OffThread)
		m.Function("release", func() { close(release) })
		m.AsyncFunction("wait2", func() { <-release2 }, OffThread)
		m.Function("release2", func() { close(release2) })
	})
	done := make(chan struct{})
	var v vm.Value
	var errs []perrors.PaseratiError
	go func() {
		defer close(done)
		v, errs = p.RunCode(`
			import { wait, release, wait2, release2 } from "io";
			const pending = wait();
			await new Promise(res => setTimeout(() => { release(); res(); }, 10));
			const sig = AbortSignal.timeout(10);
			const pending2 = wait2();
			await new Promise(res => sig.addEventListener("abort", () => { release2(); res(); }));
			await pending2;
			await pending;
			"ok"
		`, RunOptions{})
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		close(release2)
		t.Fatal("timer never fired while the external op was pending")
	}
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	if v.ToString() != "ok" {
		t.Fatalf("got %s", v.ToString())
	}
}

func TestAsyncFunctionOffThreadRejectsFuncParams(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected a panic for a func parameter")
		}
	}()
	p := NewPaserati()
	p.DeclareModule("io", func(m *ModuleBuilder) {
		m.AsyncFunction("cb", func(f func()) {}, OffThread)
	})
	p.RunCode(`import { cb } from "io"; 1`, RunOptions{})
}

type point623 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func (p *point623) Norm() float64 { return p.X*p.X + p.Y*p.Y }

// Interface and Type export TypeScript types for annotations in importing
// code, instead of silently doing nothing (#623).
func TestModuleBuilderTypeExports(t *testing.T) {
	p := NewPaserati()
	p.DeclareModule("geo", func(m *ModuleBuilder) {
		m.Type("Point", point623{})
		m.Interface("Labeled", map[string]interface{}{"label": "", "weight": 0.0})
		m.Function("origin", func() *point623 { return &point623{3, 4} })
	})
	v, errs := p.RunCode(`
		import { origin, Point, Labeled } from "geo";
		const p: Point = origin();
		const l: Labeled = { label: "a", weight: 1 };
		p.x + p.norm() + l.weight
	`, RunOptions{})
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	if v.ToFloat() != 29 {
		t.Fatalf("got %s", v.ToString())
	}
	_, errs = p.RunCode(`
		import { Labeled } from "geo";
		const bad: Labeled = { label: 1, weight: 1 };
	`, RunOptions{})
	if len(errs) == 0 {
		t.Fatal("expected a type error for a mistyped Labeled")
	}
}
