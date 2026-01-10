package tests

import (
	"fmt"
	"os"
	"testing"

	"github.com/nooga/paserati/pkg/driver"
	"github.com/nooga/paserati/pkg/vm"
)

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
				// Set environment variables for this run
				os.Setenv("PASERATI_ENABLE_PROTO_CACHE", fmt.Sprintf("%v", config.protoCache))
				os.Setenv("PASERATI_DETAILED_CACHE_STATS", fmt.Sprintf("%v", config.detailStats))

				// Reload cache configuration
				vm.EnablePrototypeCache = config.protoCache
				vm.EnableDetailedCacheStats = config.detailStats

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					p := driver.NewPaserati()
					_, errs := p.RunString(tc.code)
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset cache stats
		vm.ResetExtendedStats()

		p := driver.NewPaserati()
		_, errs := p.RunString(code)
		if len(errs) > 0 {
			b.Fatalf("Evaluation failed: %v", errs)
		}

		// Print cache stats after each run (only on last iteration)
		if i == b.N-1 {
			b.Logf("Benchmark completed %d iterations", b.N)
		}
	}
}
