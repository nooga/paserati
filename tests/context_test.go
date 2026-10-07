package tests

import (
	"runtime"
	"testing"

	"github.com/nooga/paserati/pkg/driver"
	"github.com/nooga/paserati/pkg/vm"
)

func TestContextsAreIsolated(t *testing.T) {
	p := newSkipCheck()
	prog, errs := p.Precompile(`
		var n = (typeof globalThis.n === "number" ? globalThis.n : 0) + 1;
		Array.prototype.marker = (Array.prototype.marker || 0) + 1;
		[n, [].marker, typeof secret === "undefined" ? "none" : secret()].join(",")`, driver.RunOptions{})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	for i, want := range []string{"1,1,none", "1,1,s1", "1,1,none"} {
		ctx, err := p.NewContext()
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			// per-call host state bound to this context only
			ctx.DefineNativeGlobal("secret", 0, func([]vm.Value) (vm.Value, error) { return vm.NewString("s1"), nil })
		}
		v, errs := ctx.RunProgram(prog)
		if len(errs) > 0 {
			t.Fatalf("ctx %d: %v", i, errs)
		}
		if v.ToString() != want {
			t.Fatalf("ctx %d: got %q want %q", i, v.ToString(), want)
		}
	}
}

func TestContextSurvivesThrowAndIsCollectable(t *testing.T) {
	p := newSkipCheck()
	prog, errs := p.Precompile("var big = new Array(20000).fill(1); if (true) throw new Error('x')", driver.RunOptions{})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	var ms runtime.MemStats
	run := func(n int) {
		for i := 0; i < n; i++ {
			ctx, err := p.NewContext()
			if err != nil {
				t.Fatal(err)
			}
			if _, errs := ctx.RunProgram(prog); len(errs) == 0 {
				t.Fatal("expected the throw")
			}
		}
	}
	run(20)
	runtime.GC()
	runtime.ReadMemStats(&ms)
	before := ms.HeapAlloc
	run(300)
	runtime.GC()
	runtime.ReadMemStats(&ms)
	if grown := int64(ms.HeapAlloc) - int64(before); grown > 40<<20 {
		t.Fatalf("heap grew by %d MB across 300 dropped contexts: realms are not being freed", grown>>20)
	}
}
