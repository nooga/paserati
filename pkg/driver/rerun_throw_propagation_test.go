package driver

import (
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/vm"
)

// Regression test for #298: any completed top-level run on a Paserati
// instance used to leave its "<script>" frame on the VM's frame stack, so the
// next run's script frame was pushed as a nested direct-call frame. A
// synchronous throw that crossed a native boundary (a Go builtin calling back
// into JS via vm.Call and re-throwing the result) then stopped at that frame
// as if a native caller would handle it, and the script finished with no
// error reported at all.
func TestSecondRunPropagatesThrowAcrossNativeCall(t *testing.T) {
	newInstance := func(t *testing.T) *Paserati {
		p := NewPaseratiWithInitializers(builtins.GetStandardInitializers())
		p.SetSkipTypeCheck(true)
		callIt := vm.NewNativeFunction(1, false, "callIt", func(args []vm.Value) (vm.Value, error) {
			return p.GetVM().Call(args[0], vm.Undefined, nil)
		})
		g, ok := p.GetVM().GetGlobal("globalThis")
		if !ok {
			t.Fatal("globalThis not found")
		}
		g.AsPlainObject().SetOwn("callIt", callIt)
		return p
	}

	const throwing = `callIt(() => { throw new Error("boom") })`

	priors := []struct {
		name string
		run  func(p *Paserati) int
	}{
		{"none", func(p *Paserati) int { return 0 }},
		{"RunCode script", func(p *Paserati) int {
			_, errs := p.RunCode(`1+1`, RunOptions{Script: true})
			return len(errs)
		}},
		{"RunCode module", func(p *Paserati) int {
			_, errs := p.RunCode(`1+1`, RunOptions{})
			return len(errs)
		}},
		{"EvalCode", func(p *Paserati) int {
			_, errs := p.EvalCode(`1+1`, false)
			return len(errs)
		}},
		{"RunCode x3", func(p *Paserati) int {
			n := 0
			for i := 0; i < 3; i++ {
				_, errs := p.RunCode(`1+1`, RunOptions{Script: i%2 == 0})
				n += len(errs)
			}
			return n
		}},
	}

	for _, prior := range priors {
		for _, script := range []bool{false, true} {
			name := prior.name
			if script {
				name += "/then script"
			} else {
				name += "/then module"
			}
			t.Run(name, func(t *testing.T) {
				p := newInstance(t)
				if n := prior.run(p); n != 0 {
					t.Fatalf("prior run reported %d errors", n)
				}
				_, errs := p.RunCode(throwing, RunOptions{Script: script})
				if len(errs) == 0 {
					t.Fatal("thrown error was silently lost")
				}
				if !strings.Contains(errs[0].Error(), "boom") {
					t.Fatalf("unexpected error: %v", errs[0])
				}
			})
		}
	}
}

// Closures created by a completed top-level run must keep working after the
// script frame is retired: the fix for #298 hands the frame's register window
// back, so the run's open upvalues have to be closed first.
func TestTopLevelClosuresSurviveFrameRetirement(t *testing.T) {
	p := NewPaseratiWithInitializers(builtins.GetStandardInitializers())
	p.SetSkipTypeCheck(true)

	if _, errs := p.RunCode(`
		let counter = 10;
		globalThis.bump = () => ++counter;
	`, RunOptions{Script: true}); len(errs) > 0 {
		t.Fatalf("setup errors: %v", errs)
	}
	// A second run reuses the register window the first one occupied.
	if _, errs := p.RunCode(`
		let a = 1000, b = 2000, c = 3000, d = 4000;
		a + b + c + d;
	`, RunOptions{Script: true}); len(errs) > 0 {
		t.Fatalf("second run errors: %v", errs)
	}
	val, errs := p.RunCode(`bump(); bump()`, RunOptions{Script: true})
	if len(errs) > 0 {
		t.Fatalf("bump errors: %v", errs)
	}
	if got := val.ToString(); got != "12" {
		t.Fatalf("closure lost its captured binding: got %s, want 12", got)
	}
}
