package tests

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/driver"
	"github.com/nooga/paserati/pkg/vm"
)

// compileString compiles inline benchmark source and handles errors, so
// compilation can sit above b.ResetTimer() the way compileFile does for the
// file-backed benchmarks.
func compileString(tb testing.TB, code string) *vm.Chunk {
	tb.Helper()
	chunk, compileErrs := driver.CompileString(code)
	if len(compileErrs) > 0 {
		var errMsgs strings.Builder
		for _, err := range compileErrs {
			errMsgs.WriteString(err.Error() + "\n")
		}
		tb.Fatalf("Compile errors:\n%s", errMsgs.String())
	}
	if chunk == nil {
		tb.Fatal("Compilation succeeded but returned nil chunk")
	}
	return chunk
}

// BenchmarkPrototypeMethodAccess benchmarks prototype method access performance
func BenchmarkPrototypeMethodAccess(b *testing.B) {
	// Test cases for different prototype access patterns
	testCases := []struct {
		name string
		code string
	}{
		{
			name: "StringPrototypeMethod",
			code: `
let str = "hello";
str.length;`,
		},
		{
			name: "ArrayPrototypeMethod",
			code: `
let arr = [1, 2, 3, 4, 5];
arr.length;`,
		},
		{
			name: "ObjectPrototypeChain",
			code: `
function Base() {
	this.baseValue = 10;
}
Base.prototype.getValue = function() {
	return this.baseValue;
};

let obj = new Base();
obj.getValue();`,
		},
	}

	// Run benchmarks with different cache configurations
	configs := []struct {
		name        string
		protoCache  bool
		detailStats bool
	}{
		{"Baseline", false, false},
		{"WithPrototypeCache", true, false},
		{"WithDetailedStats", true, true},
	}

	for _, config := range configs {
		for _, tc := range testCases {
			benchName := fmt.Sprintf("%s/%s", config.name, tc.name)
			b.Run(benchName, func(b *testing.B) {
				// Order matters: the cache configuration has to be applied before
				// the engine is constructed, because NewPaserati reads these when
				// it builds the VM. Hoisting construction without hoisting the
				// config first would measure the wrong configuration.
				os.Setenv("PASERATI_ENABLE_PROTO_CACHE", fmt.Sprintf("%v", config.protoCache))
				os.Setenv("PASERATI_DETAILED_CACHE_STATS", fmt.Sprintf("%v", config.detailStats))

				// Reload cache configuration
				vm.EnablePrototypeCache = config.protoCache
				vm.EnableDetailedCacheStats = config.detailStats

				// Build and compile once. Both used to happen inside the loop, so
				// every iteration paid for an engine and a compile and the
				// prototype access this benchmark is named for was a rounding
				// error on top (nooga#51). Reusing the engine also lets the
				// prototype cache warm, which is the only way Baseline and
				// WithPrototypeCache can separate at all.
				chunk := compileString(b, tc.code)
				p := driver.NewPaserati()

				// Warm the caches before timing. Hoisting construction alone left
				// the first iteration paying for cold prototype and inline caches;
				// against a ~500ns steady state that made ns/op swing from 136,792
				// at b.N=1 to 297 at b.N=4096, a worse N-dependence than the one
				// being fixed.
				//
				// The count is deliberately a round number and NOT tuned against
				// measurement: a warmup sweep on a loaded laptop returned
				// non-monotone garbage (warmup=500 reading 3x slower at b.N=16 than
				// warmup=25), which is the same single-sample trap that has cost
				// this project retractions before. 100 is cheap — ~50us per b.Run —
				// and comfortably past where the caches can still be cold. Whether
				// ns/op is actually flat in N has to be confirmed on a quiet host.
				for i := 0; i < 100; i++ {
					if _, errs := p.InterpretChunk(chunk); len(errs) > 0 {
						b.Fatalf("Warmup failed: %v", errs)
					}
				}

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					_, errs := p.InterpretChunk(chunk)
					if len(errs) > 0 {
						b.Fatalf("Evaluation failed: %v", errs)
					}
				}
			})
		}
	}
}

// BenchmarkPrototypeCacheHitRate measures cache hit rates for prototype access
func BenchmarkPrototypeCacheHitRate(b *testing.B) {
	code := `
let a = "hello";
a.length;`

	// Enable detailed cache stats for this benchmark
	os.Setenv("PASERATI_ENABLE_PROTO_CACHE", "true")
	os.Setenv("PASERATI_DETAILED_CACHE_STATS", "true")
	vm.EnablePrototypeCache = true
	vm.EnableDetailedCacheStats = true

	// Same hoist as above (nooga#51). The stats reset moves out of the loop too:
	// it is bookkeeping rather than workload, and accumulating across iterations
	// is what makes the reported rate a hit rate rather than a first-access
	// measurement repeated N times.
	chunk := compileString(b, code)
	p := driver.NewPaserati()

	// Warm before timing, for the same reason and with the same caveat as above.
	// The stats reset comes after the warmup so the reported rate covers the
	// timed iterations only.
	for i := 0; i < 100; i++ {
		if _, errs := p.InterpretChunk(chunk); len(errs) > 0 {
			b.Fatalf("Warmup failed: %v", errs)
		}
	}
	vm.ResetExtendedStats()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, errs := p.InterpretChunk(chunk)
		if len(errs) > 0 {
			b.Fatalf("Evaluation failed: %v", errs)
		}
	}
}
