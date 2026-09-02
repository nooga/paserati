package driver

import (
	"testing"
	"time"

	"github.com/nooga/paserati/pkg/vm"
)

// TestAwaitGoroutineResolvedPromiseRace is a regression test for a data
// race in the VM's core Promise machinery: resolving/settling a Promise
// from a goroutine other than the one executing the `await` opcode - which
// is exactly what fetch() does today (doFetchRequestWithContext in
// pkg/builtins/fetch_init.go calls vm.ResolvePromise from its own request
// goroutine) - used to race unsynchronized against `await`'s own direct
// reads of that promise's State/Result in pkg/vm/vm.go. `go test -race`
// caught this with nothing more than a bare pending promise, a goroutine
// calling ResolvePromise, and a naive polling read - no fetch() or any
// other feature involved.
//
// This test drives the *real* `await` opcode (both the in-async-function
// path and the top-level-await path), not a hand-rolled polling loop like
// the minimal repro, by registering a native module function that hands
// back a genuinely pending Promise which a background goroutine resolves a
// few milliseconds later - the same shape a real streaming/async host
// integration takes.
func TestAwaitGoroutineResolvedPromiseRace(t *testing.T) {
	p := NewPaserati()
	vmInstance := p.GetVM()

	p.DeclareModule("race-test", func(m *ModuleBuilder) {
		m.Function("makeGoroutineFedPromise", func() vm.Value {
			promiseVal := vmInstance.NewPendingPromise()
			promise := promiseVal.AsPromise()
			rt := vmInstance.GetAsyncRuntime()
			rt.BeginExternalOp()
			go func() {
				time.Sleep(5 * time.Millisecond)
				vmInstance.ResolvePromise(promise, vm.NumberValue(42))
				rt.EndExternalOp()
			}()
			return promiseVal
		})
	})

	t.Run("await inside an async function", func(t *testing.T) {
		tsCode := `
			import { makeGoroutineFedPromise } from "race-test";

			async function main(): Promise<number> {
				const v = await makeGoroutineFedPromise();
				return v + 1;
			}

			await main();
		`
		result, errs := p.RunStringWithModules(tsCode)
		if len(errs) > 0 {
			t.Fatalf("script failed: %v", errs[0])
		}
		if result.ToFloat() != 43 {
			t.Fatalf("expected 43, got %v", result.Inspect())
		}
	})

	t.Run("top-level await", func(t *testing.T) {
		tsCode := `
			import { makeGoroutineFedPromise } from "race-test";
			const v = await makeGoroutineFedPromise();
			v + 1;
		`
		result, errs := p.RunStringWithModules(tsCode)
		if len(errs) > 0 {
			t.Fatalf("script failed: %v", errs[0])
		}
		if result.ToFloat() != 43 {
			t.Fatalf("expected 43, got %v", result.Inspect())
		}
	})
}
