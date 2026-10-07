package driver

import (
	"runtime"
	"sync"
	"testing"
)

func heapAlloc() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// Dropped instances must not leave anything reachable from package-level
// state. Each one used to add its own chain to the global shape tree, about
// 17-30 KB per instance (#605).
func TestDroppedInstancesDoNotLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates a few hundred instances")
	}
	run := func(n int) {
		for i := 0; i < n; i++ {
			p := NewPaserati()
			p.RunCode("1+1", RunOptions{})
			p.Cleanup()
		}
	}
	run(50) // warm up process-wide caches
	before := heapAlloc()
	run(300)
	after := heapAlloc()
	if after > before && after-before > 2<<20 {
		t.Fatalf("heap grew by %d KB across 300 dropped instances", (after-before)>>10)
	}
}

// Independent instances on separate goroutines share no mutable package
// state; run with -race (#606).
func TestConcurrentInstances(t *testing.T) {
	const src = `class A { m(): number { return 1 } } const xs: number[] = [1,2,3];
		new WritableStream(); new ReadableStream(); new TransformStream();
		xs.map(x => x * new A().m()).join(",") + /a(b)/.exec("ab")![1] + String(Symbol.iterator in [])`
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 2; i++ {
				p := NewPaserati()
				p.SetSkipTypeCheck(g%2 == 0)
				v, errs := p.RunCode(src, RunOptions{})
				if len(errs) > 0 {
					t.Errorf("RunCode: %v", errs[0])
					return
				}
				if got := v.ToString(); got != "1,2,3btrue" {
					t.Errorf("got %q", got)
				}
			}
		}(g)
	}
	wg.Wait()
}
