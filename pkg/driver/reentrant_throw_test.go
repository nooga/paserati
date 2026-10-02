package driver

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/vm"
)

// A JS throw that crosses two nested vm.Call boundaries (what noderati's
// require() inside require() does: each file body runs under its own vm.Call,
// the second from a native function called by the first) must arrive as the
// real thrown value, caught or not (#142).
func TestThrowAcrossNestedVMCall(t *testing.T) {
	newInstance := func() *Paserati {
		p := NewPaseratiWithInitializers(builtins.GetStandardInitializers())
		p.SetSkipTypeCheck(true)
		callIt := vm.NewNativeFunction(1, false, "callIt", func(args []vm.Value) (vm.Value, error) {
			return p.GetVM().Call(args[0], vm.Undefined, nil)
		})
		// requireLike re-reports a failed nested call as a plain Go error
		// (not an ExceptionError), as noderati's require() does.
		requireLike := vm.NewNativeFunction(1, false, "requireLike", func(args []vm.Value) (vm.Value, error) {
			v, err := p.GetVM().Call(args[0], vm.Undefined, nil)
			if err != nil {
				return vm.Undefined, fmt.Errorf("require failed: %s", err.Error())
			}
			return v, nil
		})
		g, _ := p.GetVM().GetGlobal("globalThis")
		g.AsPlainObject().SetOwn("callIt", callIt)
		g.AsPlainObject().SetOwn("requireLike", requireLike)
		return p
	}

	t.Run("uncaught reports the real error", func(t *testing.T) {
		p := newInstance()
		_, errs := p.RunCode(`callIt(() => callIt(() => { throw new Error("boom from inner") }))`, RunOptions{Script: true})
		if len(errs) == 0 {
			t.Fatal("no error reported")
		}
		if msg := errs[0].Error(); !strings.Contains(msg, "boom from inner") {
			t.Errorf("error = %q, want it to mention the thrown message", msg)
		}
	})

	t.Run("caught value is the real Error", func(t *testing.T) {
		p := newInstance()
		v, errs := p.RunCode(`
let out;
try { callIt(() => callIt(() => { throw new Error("boom from inner") })); }
catch (e) { out = [typeof e, e === null, e && e.message, e instanceof Error].join(","); }
out;`, RunOptions{Script: true})
		if len(errs) > 0 {
			t.Fatalf("unexpected error: %v", errs[0])
		}
		if got, want := v.ToString(), "object,false,boom from inner,true"; got != want {
			t.Errorf("caught = %q, want %q", got, want)
		}
	})

	t.Run("caught between the two levels", func(t *testing.T) {
		p := newInstance()
		v, errs := p.RunCode(`
let out;
callIt(() => {
  try { callIt(() => { throw new RangeError("deep") }); } catch (e) { out = e.name + ":" + e.message; }
});
out;`, RunOptions{Script: true})
		if len(errs) > 0 {
			t.Fatalf("unexpected error: %v", errs[0])
		}
		if got, want := v.ToString(), "RangeError:deep"; got != want {
			t.Errorf("caught = %q, want %q", got, want)
		}
	})

	t.Run("plain Go error from a nested native is catchable, not null", func(t *testing.T) {
		p := newInstance()
		v, errs := p.RunCode(`
let out;
try { callIt(() => requireLike(() => { throw new Error("boom from b") })); }
catch (e) { out = [e === null, typeof e, e && e.message].join(","); }
out;`, RunOptions{Script: true})
		if len(errs) > 0 {
			t.Fatalf("unexpected error: %v", errs[0])
		}
		got := v.ToString()
		if strings.HasPrefix(got, "true") || !strings.Contains(got, "require failed") {
			t.Errorf("caught = %q, want a non-null error carrying the require failure", got)
		}
	})
}
