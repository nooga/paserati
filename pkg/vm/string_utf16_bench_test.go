package vm

import (
	"strings"
	"testing"
)

// BenchmarkCharCodeAtScan models TypeScript's scanner: a forward loop calling
// charCodeAt over one long string. Before the classification cache this was
// O(n) per call (a fresh []uint16 materialisation), i.e. O(n^2) for the scan.
func BenchmarkCharCodeAtScan(b *testing.B) {
	s := strings.Repeat("x", 100_000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var acc uint64
		n := UTF16Length(s)
		for j := 0; j < n; j++ {
			u, _ := UTF16CodeUnitAt(s, j)
			acc += uint64(u)
		}
		if acc == 0 {
			b.Fatal("unexpected zero")
		}
	}
}

func BenchmarkCharCodeAtScanNonASCII(b *testing.B) {
	s := strings.Repeat("é", 50_000) // 2 bytes each, 50k code units
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var acc uint64
		n := UTF16Length(s)
		for j := 0; j < n; j++ {
			u, _ := UTF16CodeUnitAt(s, j)
			acc += uint64(u)
		}
		if acc == 0 {
			b.Fatal("unexpected zero")
		}
	}
}
