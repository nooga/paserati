package driver

import (
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

// BenchmarkVariadicEmptyRestCall measures the paserati A4 fix's stated cost
// (docs/runtime-production-roadmap.md, A4's acceptance criteria: "Record any
// allocation increase as the cost of the semantic repair... Ordinary call
// performance is a required canary"). The fix removed a shared, mutable
// `emptyRestArray` singleton that every variadic call with no extra
// arguments used to return, replacing it with a fresh NewArray() per call so
// two callers' empty rest arrays are no longer the same identity. That turns
// what used to be a zero-allocation path (for the empty-rest case
// specifically) into a one-array-allocation path on every such call.
//
// This benchmark isolates exactly that call shape - a variadic function
// called with zero extra arguments - compiled once and invoked repeatedly
// through vm.Call so the reported allocs/op reflect the call path itself,
// not script compilation or an enclosing JS loop's own overhead.
func BenchmarkVariadicEmptyRestCall(b *testing.B) {
	p := NewPaserati()
	if _, errs := p.RunString(`function f(...args) { return args; }`); len(errs) > 0 {
		b.Fatalf("setup failed: %v", errs)
	}
	fn, ok := p.GetVM().GetGlobal("f")
	if !ok {
		b.Fatal("global function f not found after setup")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.GetVM().Call(fn, vm.Undefined, nil); err != nil {
			b.Fatalf("call failed: %v", err)
		}
	}
}
