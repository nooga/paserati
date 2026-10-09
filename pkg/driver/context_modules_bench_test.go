package driver

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/modules"
)

// graphSources builds a module graph of n modules: m0 is a leaf and each mK
// imports m(K-1). Every module has functions, a class and top-level state, so
// the graph is a few hundred lines per module-ish of ordinary code.
func graphSources(prefix string, n int) map[string]string {
	out := make(map[string]string, n)
	for i := 0; i < n; i++ {
		var b strings.Builder
		if i > 0 {
			fmt.Fprintf(&b, "import { total as prev } from \"%s%d\";\n", prefix, i-1)
		} else {
			b.WriteString("const prev = 0;\n")
		}
		fmt.Fprintf(&b, "let calls = 0;\nexport interface Item%d { id: number; name: string }\n", i)
		for f := 0; f < 8; f++ {
			fmt.Fprintf(&b, "export function f%d_%d(x: number): number { calls++; let s = x; for (let i = 0; i < 3; i++) { s += i * %d; } return s + prev; }\n", i, f, f+1)
		}
		fmt.Fprintf(&b, "export class C%d { items: Item%d[] = []; add(id: number, name: string): this { this.items.push({ id, name }); return this; } get size(): number { return this.items.length; } }\n", i, i)
		fmt.Fprintf(&b, "export const total: number = prev + f%d_0(1) + new C%d().add(1, \"a\").size;\n", i, i)
		out[fmt.Sprintf("%s%d", prefix, i)] = b.String()
	}
	return out
}

func newGraphSession(prefix string, n int) *Paserati {
	p := NewPaserati()
	mem := modules.NewMemoryResolver("mem" + prefix)
	for name, src := range graphSources(prefix, n) {
		mem.AddModule(name, src)
	}
	p.AddResolver(mem)
	return p
}

const graphModules = 20

func graphEntry(prefix string) string {
	return fmt.Sprintf(`import { total, f%d_3 } from "%s%d"; total + f%d_3(2)`, graphModules-1, prefix, graphModules-1, graphModules-1)
}

// One call on a fresh session: parse, check and compile the graph every time.
func BenchmarkModuleGraphFreshSession(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p := newGraphSession("g", graphModules)
		if _, errs := p.RunCode(graphEntry("g"), RunOptions{ModuleName: "/e.ts", Filename: "/e.ts"}); len(errs) > 0 {
			b.Fatal(errs)
		}
		p.Cleanup()
	}
}

// One call in a new Context of a long-lived session: the graph is parsed,
// checked and compiled once, then instantiated per Context.
func BenchmarkModuleGraphNewContext(b *testing.B) {
	p := newGraphSession("g", graphModules)
	defer p.Cleanup()
	warm, _ := p.NewContext()
	if _, errs := warm.RunCode(graphEntry("g"), RunOptions{ModuleName: "/warm.ts", Filename: "/warm.ts"}); len(errs) > 0 {
		b.Fatal(errs)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c, err := p.NewContext()
		if err != nil {
			b.Fatal(err)
		}
		if _, errs := c.RunCode(graphEntry("g"), RunOptions{ModuleName: "/e.ts", Filename: "/e.ts"}); len(errs) > 0 {
			b.Fatal(errs)
		}
	}
}

// Just creating a Context, with the session holding k compiled graphs: shows
// what a realm costs as the session grows (its heap mirrors the session's
// global layout).
func BenchmarkNewContextWithGraphs(b *testing.B) {
	for _, k := range []int{0, 5, 25} {
		b.Run(fmt.Sprintf("graphs=%d", k), func(b *testing.B) {
			p := NewPaserati()
			defer p.Cleanup()
			for g := 0; g < k; g++ {
				prefix := fmt.Sprintf("g%d_", g)
				mem := modules.NewMemoryResolver("mem" + prefix)
				for name, src := range graphSources(prefix, graphModules) {
					mem.AddModule(name, src)
				}
				p.AddResolver(mem)
				c, _ := p.NewContext()
				if _, errs := c.RunCode(graphEntry(prefix), RunOptions{ModuleName: "/w.ts", Filename: "/w.ts"}); len(errs) > 0 {
					b.Fatal(errs)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := p.NewContext(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestContextRetainedMemory reports what a Context that ran one graph keeps
// alive (kept as a test so it runs under -v on demand).
func TestContextRetainedMemory(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	p := newGraphSession("g", graphModules)
	defer p.Cleanup()
	c0, _ := p.NewContext()
	if _, errs := c0.RunCode(graphEntry("g"), RunOptions{ModuleName: "/warm.ts", Filename: "/warm.ts"}); len(errs) > 0 {
		t.Fatal(errs)
	}
	const n = 50
	ctxs := make([]*Context, 0, n)
	var before, after runtime.MemStats
	retained := func(run func(c *Context)) uint64 {
		runtime.GC()
		runtime.ReadMemStats(&before)
		held := make([]*Context, 0, n)
		for i := 0; i < n; i++ {
			c, _ := p.NewContext()
			run(c)
			held = append(held, c)
		}
		runtime.GC()
		runtime.ReadMemStats(&after)
		runtime.KeepAlive(held)
		return (after.HeapAlloc - before.HeapAlloc) / n / 1024
	}
	empty := retained(func(c *Context) {})
	t.Logf("per Context with nothing run: %d KB retained", empty)
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < n; i++ {
		c, _ := p.NewContext()
		if _, errs := c.RunCode(graphEntry("g"), RunOptions{ModuleName: "/e.ts", Filename: "/e.ts"}); len(errs) > 0 {
			t.Fatal(errs)
		}
		ctxs = append(ctxs, c)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	t.Logf("per Context after one %d-module graph call: %d KB retained", graphModules, (after.HeapAlloc-before.HeapAlloc)/n/1024)
	runtime.KeepAlive(ctxs)
}

// TestSessionRetainedPerGraph reports what a session keeps for each compiled
// module graph (what the loader caches once, for every Context to share).
func TestSessionRetainedPerGraph(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	p := NewPaserati()
	defer p.Cleanup()
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	const k = 5
	for g := 0; g < k; g++ {
		prefix := fmt.Sprintf("g%d_", g)
		mem := modules.NewMemoryResolver("mem" + prefix)
		for name, src := range graphSources(prefix, graphModules) {
			mem.AddModule(name, src)
		}
		p.AddResolver(mem)
		c, _ := p.NewContext()
		if _, errs := c.RunCode(graphEntry(prefix), RunOptions{ModuleName: "/w.ts", Filename: "/w.ts"}); len(errs) > 0 {
			t.Fatal(errs)
		}
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	src := 0
	for _, s := range graphSources("g", graphModules) {
		src += len(s)
	}
	if path := os.Getenv("HEAP_PROFILE"); path != "" {
		f, _ := os.Create(path)
		pprof.WriteHeapProfile(f)
		f.Close()
	}
	t.Logf("session keeps %d KB per %d-module graph (%d KB of source)", (after.HeapAlloc-before.HeapAlloc)/k/1024, graphModules, src/1024)
}

// A dropped Context takes its module instances with it: nothing in the session
// (the module loader, the VM) keeps a realm's module state alive.
func TestContextModulesAreCollected(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	p := newGraphSession("g", graphModules)
	defer p.Cleanup()
	run := func() {
		c, _ := p.NewContext()
		if _, errs := c.RunCode(graphEntry("g"), RunOptions{ModuleName: "/e.ts", Filename: "/e.ts"}); len(errs) > 0 {
			t.Fatal(errs)
		}
	}
	run() // loads and compiles the graph
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	const n = 100
	for i := 0; i < n; i++ {
		run()
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	grew := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("heap grew %d KB over %d dropped Contexts", grew/1024, n)
	if grew > 8<<20 { // retaining them would be ~70 MB
		t.Fatalf("heap grew %d KB over %d dropped Contexts: they are being retained", grew/1024, n)
	}
}
