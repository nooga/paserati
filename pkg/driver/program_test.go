package driver

import (
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

// The shared-state tests in tests/ pass even if RunProgram silently falls back
// to compiling from source, so pin down that ordinary programs really are
// instantiated from the precompiled chunk.
func TestOrdinaryProgramsAreInstantiable(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)
	prog, errs := p.Precompile("function f(x){return ()=>x}; class A{ m(){return 1} }; f(1)() + new A().m()", RunOptions{})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	a, ok := vm.InstantiateChunk(prog.chunk)
	if !ok {
		t.Fatal("expected the program to be instantiable")
	}
	b, _ := vm.InstantiateChunk(prog.chunk)
	if a == b || &a.Constants[0] == &b.Constants[0] {
		t.Fatal("instances must not share their constant pools")
	}
}
