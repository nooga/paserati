package tests

import (
	"testing"

	"github.com/nooga/paserati/pkg/driver"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/vm"
)

const programSrc = `
var counter = 0;
function bump() { counter++; return counter; }
class Box { constructor(v) { this.v = v; } get() { return this.v; } }
bump.extra = (bump.extra || 0) + 1;     // property on a function object: must not leak between runs
Box.prototype.tag = "t" + counter;      // nor on a class prototype
const out = [bump(), bump(), new Box(bump()).get(), bump.extra, typeof leaked];
globalThis.leaked = true;
out.join(",");
`

func newSkipCheck() *driver.Paserati {
	p := driver.NewPaserati()
	p.SetSkipTypeCheck(true)
	return p
}

func TestProgramRunsOnFreshSessionsWithoutSharingState(t *testing.T) {
	prog, errs := newSkipCheck().Precompile(programSrc, driver.RunOptions{Filename: "prog.js"})
	if len(errs) > 0 {
		t.Fatalf("precompile: %v", errs)
	}
	const want = "1,2,3,1,undefined"
	for i := 0; i < 3; i++ {
		p := newSkipCheck()
		v, errs := p.RunProgram(prog)
		if len(errs) > 0 {
			t.Fatalf("run %d: %v", i, errs)
		}
		if got := v.ToString(); got != want {
			t.Fatalf("run %d on a fresh session: got %q want %q", i, got, want)
		}
	}
}

func TestProgramRunsRepeatedlyOnOneSessionAndMatchesRunCode(t *testing.T) {
	p := newSkipCheck()
	prog, errs := p.Precompile("let n = 40; n + 2", driver.RunOptions{})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	v, errs := p.RunProgram(prog)
	if len(errs) > 0 || v.ToString() != "42" {
		t.Fatalf("got %v %v", v.ToString(), errs)
	}
	// A throwing program leaves the session usable (#594) and reusable.
	bad, errs := p.Precompile("throw new Error('boom')", driver.RunOptions{})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	for i := 0; i < 2; i++ {
		if _, errs := p.RunProgram(bad); len(errs) == 0 {
			t.Fatal("expected an error")
		}
		if v, errs := newSkipCheck().RunProgram(bad); len(errs) == 0 {
			t.Fatalf("expected an error on a fresh session, got %v", v.ToString())
		}
	}
}

func TestPrecompileReportsErrors(t *testing.T) {
	if _, errs := newSkipCheck().Precompile("let = = 1", driver.RunOptions{}); len(errs) == 0 {
		t.Fatal("expected a parse error")
	}
}

// Array holes ([, 1]) put a TypeHole constant in the pool. It is an immutable
// sentinel, so the program must still instantiate rather than silently fall
// back to recompiling from source on every run.
func TestInstantiateChunkWithArrayHole(t *testing.T) {
	p := driver.NewPaserati()
	prog, errs := parser.NewParser(lexer.NewLexer("var a = [, 1]; a.length")).ParseProgram()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	chunk, cerrs := p.CompileProgramAsScript(prog)
	if len(cerrs) > 0 {
		t.Fatal(cerrs)
	}
	if _, ok := vm.InstantiateChunk(chunk); !ok {
		t.Fatal("chunk with an array hole should be instantiable")
	}
	prog2, perrs := p.Precompile("var a = [, 1]; a.length", driver.RunOptions{})
	if len(perrs) > 0 {
		t.Fatal(perrs)
	}
	if !prog2.Instantiable() {
		t.Fatal("Program with an array hole should report Instantiable")
	}
}
