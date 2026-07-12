package main

import (
	"testing"
	"time"

	"github.com/nooga/paserati/pkg/test262"
)

func res(path string, passed, timedOut bool, d time.Duration) test262.Result {
	return test262.Result{Path: path, Passed: passed, TimedOut: timedOut, Duration: d}
}

// totalRecord returns the "test262.total" record from a build.
func totalRecord(t *testing.T, out test262.Output) (nsPerOp float64, iters int64, setHash string) {
	t.Helper()
	for _, r := range buildRecords(out, "2026-07-12T00:00:00Z") {
		if r.Name == "total" {
			return r.NSPerOp, r.Iterations, r.SetHash
		}
	}
	t.Fatal("no total record emitted")
	return 0, 0, ""
}

// The core F3 property: two runs with the SAME passing count but a DIFFERENT
// set must get different SetHashes, so the total isn't silently comparable.
func TestSetHashDistinguishesEqualCountDifferentSet(t *testing.T) {
	runA := test262.Output{Results: []test262.Result{
		res("test262/test/built-ins/Math/abs/a.js", true, false, 10),
		res("test262/test/built-ins/Math/ceil/b.js", true, false, 20),
	}}
	// Same count (2), same summed ns (30), but c.js replaces ceil/b.js.
	runB := test262.Output{Results: []test262.Result{
		res("test262/test/built-ins/Math/abs/a.js", true, false, 10),
		res("test262/test/built-ins/Math/floor/c.js", true, false, 20),
	}}

	nsA, cntA, hA := totalRecord(t, runA)
	nsB, cntB, hB := totalRecord(t, runB)

	if cntA != cntB || nsA != nsB {
		t.Fatalf("precondition: expected equal count/sum, got A(%d,%.0f) B(%d,%.0f)", cntA, nsA, cntB, nsB)
	}
	if hA == "" || hB == "" {
		t.Fatalf("expected non-empty set hashes, got %q %q", hA, hB)
	}
	if hA == hB {
		t.Fatalf("equal-count different-set runs must hash differently; both = %q", hA)
	}
}

// The hash is order-independent (it's a set) and stable across identical runs.
func TestSetHashIsOrderIndependentAndStable(t *testing.T) {
	forward := test262.Output{Results: []test262.Result{
		res("test262/test/language/a.js", true, false, 5),
		res("test262/test/language/b.js", true, false, 7),
		res("test262/test/language/c.js", true, false, 9),
	}}
	reversed := test262.Output{Results: []test262.Result{
		res("test262/test/language/c.js", true, false, 9),
		res("test262/test/language/b.js", true, false, 7),
		res("test262/test/language/a.js", true, false, 5),
	}}

	_, _, hF := totalRecord(t, forward)
	_, _, hR := totalRecord(t, reversed)
	if hF != hR {
		t.Fatalf("set hash must be order-independent: %q != %q", hF, hR)
	}
}

// Failing and timed-out tests are excluded from both the sum and the set,
// matching the metric's definition.
func TestSetHashExcludesFailedAndTimedOut(t *testing.T) {
	withNoise := test262.Output{Results: []test262.Result{
		res("test262/test/built-ins/Math/abs/a.js", true, false, 10),
		res("test262/test/built-ins/Math/ceil/b.js", false, false, 20), // failed
		res("test262/test/built-ins/Math/floor/c.js", true, true, 30),  // timed out
	}}
	clean := test262.Output{Results: []test262.Result{
		res("test262/test/built-ins/Math/abs/a.js", true, false, 10),
	}}

	nsN, cntN, hN := totalRecord(t, withNoise)
	nsC, cntC, hC := totalRecord(t, clean)
	if cntN != 1 || nsN != 10 {
		t.Fatalf("expected only the one passing test to count, got count=%d ns=%.0f", cntN, nsN)
	}
	if hN != hC || cntN != cntC || nsN != nsC {
		t.Fatalf("passing-only set must match the clean run: h %q vs %q", hN, hC)
	}
}
