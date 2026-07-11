package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A minimal anchor pair — aggregateFromFile errors out without a positive-ns
// BenchmarkRatchetAnchor to normalize against. Micro benchmarks carry no
// set_hash, so the anchor never participates in the fingerprint logic.
const anchorJSONL = `{"package":"github.com/nooga/paserati/pkg/vm","name":"BenchmarkRatchetAnchor","iterations":1000,"ns_per_op":1.0,"bytes_per_op":0,"allocs_per_op":0,"captured_at":"2026-07-13T00:00:00Z"}
`

func aggregate(t *testing.T, jsonl string) Baseline {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.jsonl")
	if err := os.WriteFile(path, []byte(anchorJSONL+jsonl), 0o644); err != nil {
		t.Fatal(err)
	}
	base, err := aggregateFromFile(path)
	if err != nil {
		t.Fatalf("aggregateFromFile: %v", err)
	}
	return base
}

const macroKey = "github.com/nooga/paserati/pkg/test262.BenchmarkTest262"

// TestAggregateBlanksConflictingSetHash covers the one branch in
// aggregateFromFile without other coverage: when the -count repetitions of a
// macro benchmark disagree on set_hash, the aggregated identity must blank to
// "" — an honest "no coherent set" signal — rather than latch the first hash.
func TestAggregateBlanksConflictingSetHash(t *testing.T) {
	jsonl := `{"package":"github.com/nooga/paserati/pkg/test262","name":"BenchmarkTest262","iterations":10,"ns_per_op":100.0,"bytes_per_op":0,"allocs_per_op":0,"set_hash":"aaaa","captured_at":"2026-07-13T00:00:00Z"}
{"package":"github.com/nooga/paserati/pkg/test262","name":"BenchmarkTest262","iterations":10,"ns_per_op":110.0,"bytes_per_op":0,"allocs_per_op":0,"set_hash":"bbbb","captured_at":"2026-07-13T00:00:00Z"}
`
	e, ok := aggregate(t, jsonl).Benchmarks[macroKey]
	if !ok {
		t.Fatalf("benchmark %q missing from baseline", macroKey)
	}
	if e.SetHash != "" {
		t.Errorf("SetHash = %q, want \"\" (records disagreed → no coherent identity)", e.SetHash)
	}
}

// TestAggregatePreservesAgreeingSetHash is the complement: repetitions that all
// report the same set_hash keep it, so a genuine set identity survives aggregation.
func TestAggregatePreservesAgreeingSetHash(t *testing.T) {
	jsonl := `{"package":"github.com/nooga/paserati/pkg/test262","name":"BenchmarkTest262","iterations":10,"ns_per_op":100.0,"bytes_per_op":0,"allocs_per_op":0,"set_hash":"aaaa","captured_at":"2026-07-13T00:00:00Z"}
{"package":"github.com/nooga/paserati/pkg/test262","name":"BenchmarkTest262","iterations":10,"ns_per_op":110.0,"bytes_per_op":0,"allocs_per_op":0,"set_hash":"aaaa","captured_at":"2026-07-13T00:00:00Z"}
`
	e, ok := aggregate(t, jsonl).Benchmarks[macroKey]
	if !ok {
		t.Fatalf("benchmark %q missing from baseline", macroKey)
	}
	if e.SetHash != "aaaa" {
		t.Errorf("SetHash = %q, want \"aaaa\" (agreeing records preserve identity)", e.SetHash)
	}
}

// TestAggregateFromFileUsesMin verifies that multiple -count repetitions of the
// same benchmark are reduced by minimum, not mean — the fastest sample is the
// least noise-contaminated estimate. A mean reducer here would report 115.
func TestAggregateFromFileUsesMin(t *testing.T) {
	// anchor: three clean samples; noisy: one fast + two slower (upward noise).
	jsonl := `{"package":"github.com/nooga/paserati/pkg/vm","name":"BenchmarkRatchetAnchor","iterations":1000,"ns_per_op":1.0,"bytes_per_op":0,"allocs_per_op":0,"captured_at":"2026-07-11T00:00:00Z"}
{"package":"github.com/nooga/paserati/pkg/vm","name":"BenchmarkRatchetAnchor","iterations":1000,"ns_per_op":1.0,"bytes_per_op":0,"allocs_per_op":0,"captured_at":"2026-07-11T00:00:00Z"}
{"package":"github.com/nooga/paserati/pkg/vm","name":"BenchmarkRatchetAnchor","iterations":1000,"ns_per_op":1.0,"bytes_per_op":0,"allocs_per_op":0,"captured_at":"2026-07-11T00:00:00Z"}
{"package":"github.com/nooga/paserati/pkg/vm","name":"BenchmarkNoisy","iterations":500,"ns_per_op":100.0,"bytes_per_op":8,"allocs_per_op":2,"captured_at":"2026-07-11T00:00:00Z"}
{"package":"github.com/nooga/paserati/pkg/vm","name":"BenchmarkNoisy","iterations":500,"ns_per_op":140.0,"bytes_per_op":8,"allocs_per_op":2,"captured_at":"2026-07-11T00:00:00Z"}
{"package":"github.com/nooga/paserati/pkg/vm","name":"BenchmarkNoisy","iterations":500,"ns_per_op":105.0,"bytes_per_op":8,"allocs_per_op":2,"captured_at":"2026-07-11T00:00:00Z"}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.jsonl")
	if err := os.WriteFile(path, []byte(jsonl), 0o644); err != nil {
		t.Fatal(err)
	}

	base, err := aggregateFromFile(path)
	if err != nil {
		t.Fatalf("aggregateFromFile: %v", err)
	}

	const key = "github.com/nooga/paserati/pkg/vm.BenchmarkNoisy"
	e, ok := base.Benchmarks[key]
	if !ok {
		t.Fatalf("benchmark %q missing from baseline", key)
	}
	if e.NSPerOp != 100.0 {
		t.Errorf("NSPerOp = %.1f, want 100.0 (min of {100,140,105}; mean would be 115)", e.NSPerOp)
	}
	// Anchor is min-reduced too (all 1.0 here) and normalizes the ratio.
	if e.RatioToAnchor != 100.0 {
		t.Errorf("RatioToAnchor = %.1f, want 100.0", e.RatioToAnchor)
	}
	if e.AllocsPerOp != 2 || e.BytesPerOp != 8 {
		t.Errorf("allocs/bytes = %d/%d, want 2/8", e.AllocsPerOp, e.BytesPerOp)
	}
	// All raw samples retained for provenance.
	if len(e.Samples) != 3 {
		t.Errorf("Samples len = %d, want 3", len(e.Samples))
	}
}
