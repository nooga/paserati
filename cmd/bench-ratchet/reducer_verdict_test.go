package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestReducerChoiceChangesRegressionVerdict shows that the choice of reducer
// changes the pass/fail verdict of the informational A/B, not just the number.
//
// The 15 samples below are a real capture (2026-07-12, Apple M2):
//
//	go test ./pkg/vm -bench 'BenchmarkGetOwn/n=16/last|BenchmarkRatchetAnchor' -count=15
//
// and are deliberately a REGIME TRANSITION: five fast runs (~9.8) then ten slow
// ones (~12.7) — min 9.536, mean 11.759, median 12.370. That is *not* tidy iid
// noise, which is exactly the point. The minimum selects the fast-regime lower
// envelope; the mean is dragged into the slow regime by the tail. Fed through
// the real compareAndReport at perf-pr's 10% budget, against a baseline pinned
// at the lower envelope:
//
//	min-reduced  ratio 9.536 / 1.172 = 8.14  -> 0 regressions
//	mean-reduced ratio 11.759 / 1.243 = 9.46 -> 1 regression (+16%)
//
// So the reducer alone flips the verdict. What this does and does NOT show: it
// demonstrates verdict SUPPRESSION under an upward-contamination assumption — it
// does not prove the suppressed result was phantom. A sustained slow regime can
// be real; telling environmental drift from a code change needs interleaved runs
// (the median-of-N repeat gate), not this reducer. min-of-N here is a pragmatic
// lower-envelope heuristic for the informational report, never a gate.
func TestReducerChoiceChangesRegressionVerdict(t *testing.T) {
	anchor := []float64{1.172, 1.217, 1.208, 1.199, 1.215, 1.196, 1.199, 1.199, 1.196, 1.187, 1.190, 1.192, 1.478, 1.430, 1.373}
	family := []float64{9.751, 9.743, 9.536, 9.922, 10.14, 13.38, 12.38, 12.74, 12.51, 12.17, 12.82, 13.45, 13.39, 12.37, 12.08}
	const (
		famKey = "github.com/nooga/paserati/pkg/vm.BenchmarkGetOwn/n=16/last"
		budget = 0.10 // perf-pr.yml runs bench-ratchet at 10%
	)

	// 1. Reduce the raw samples through the real aggregate path; confirm it's min.
	path := filepath.Join(t.TempDir(), "runs.jsonl")
	if err := os.WriteFile(path, []byte(buildRunsJSONL(anchor, family)), 0o644); err != nil {
		t.Fatal(err)
	}
	curMin, err := aggregateFromFile(path, "min")
	if err != nil {
		t.Fatalf("aggregateFromFile: %v", err)
	}
	wantMinRatio := minOf(family) / minOf(anchor)
	if got := curMin.Benchmarks[famKey].RatioToAnchor; !approxEq(got, wantMinRatio) {
		t.Fatalf("aggregated ratio_to_anchor = %.4f, want %.4f (min-reduced)", got, wantMinRatio)
	}

	// 2. Accepted baseline = the lower envelope (min). Same machine => no fingerprint warning.
	baseline := Baseline{
		Machine:    curMin.Machine,
		Reducer:    curMin.Reducer,
		Anchor:     AnchorRecord{NSPerOp: minOf(anchor)},
		Benchmarks: map[string]BenchmarkEntry{famKey: {RatioToAnchor: wantMinRatio}},
	}

	// 3. Same samples, mean reducer: the ratio inflates past budget.
	curMean := Baseline{
		Machine:    curMin.Machine,
		Reducer:    curMin.Reducer,
		Anchor:     AnchorRecord{NSPerOp: meanOf(anchor)},
		Benchmarks: map[string]BenchmarkEntry{famKey: {RatioToAnchor: meanOf(family) / meanOf(anchor)}},
	}

	// 4. Drive the real regression check. Reducer choice alone flips the verdict.
	if got := quietRegressions(baseline, curMin, budget); got != 0 {
		t.Errorf("min reducer: %d regression(s), want 0 — baseline is pinned at this lower envelope", got)
	}
	if got := quietRegressions(baseline, curMean, budget); got != 1 {
		t.Errorf("mean reducer: %d regression(s), want 1 — the verdict min suppresses", got)
	}
}

// TestMinReducerRobustAtProductionCount3 addresses the N-mismatch: the verdict
// proof above uses N=15, but perf-pr.yml / perf-timeline.yml ship `-count 3`.
// At the shipped count, min-of-3 must reject a single upward-contaminated sample
// (a lone GC pause / CPU migration in one of three runs), so it stays pinned to
// the fast-regime floor whether or not an outlier is present — that is what makes
// min-of-3 stable rather than a different flavor of outlier-chasing. mean-of-3, by
// contrast, moves with the outlier (the control assertion).
func TestMinReducerRobustAtProductionCount3(t *testing.T) {
	const famKey = "github.com/nooga/paserati/pkg/vm.BenchmarkGetOwn/n=16/last"
	anchor := []float64{1.172, 1.217, 1.208} // three fast anchor runs
	clean := []float64{9.751, 9.536, 9.922}  // three fast family runs
	contam := []float64{9.751, 9.536, 13.38} // same first two + one slow outlier

	ratioMin := func(anchor, family []float64) float64 {
		path := filepath.Join(t.TempDir(), "runs.jsonl")
		if err := os.WriteFile(path, []byte(buildRunsJSONL(anchor, family)), 0o644); err != nil {
			t.Fatal(err)
		}
		b, err := aggregateFromFile(path, "min")
		if err != nil {
			t.Fatalf("aggregateFromFile: %v", err)
		}
		return b.Benchmarks[famKey].RatioToAnchor
	}

	// min-of-3 is invariant to the single slow outlier: the reduced ratio is the
	// same clean-run floor with or without it.
	if a, b := ratioMin(anchor, clean), ratioMin(anchor, contam); !approxEq(a, b) {
		t.Errorf("min-of-3 moved with a single outlier: %.4f -> %.4f (want stable)", a, b)
	}
	// Control: the mean-of-3 ratio DOES move, confirming the outlier is real and
	// min's stability isn't just because the sample is inert.
	meanClean := meanOf(clean) / meanOf(anchor)
	meanContam := meanOf(contam) / meanOf(anchor)
	if approxEq(meanClean, meanContam) {
		t.Fatalf("test setup: expected mean-of-3 to move with the outlier, got %.4f == %.4f", meanClean, meanContam)
	}
}

// quietRegressions runs the real compareAndReport with its stdout report
// discarded, returning the regression count.
func quietRegressions(baseline, current Baseline, budget float64) int {
	old := os.Stdout
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err == nil {
		os.Stdout = devnull
		defer func() { os.Stdout = old; devnull.Close() }()
	}
	return compareAndReport(baseline, current, budget, "text").regressions
}

func buildRunsJSONL(anchor, family []float64) string {
	var b strings.Builder
	emit := func(name string, ns float64) {
		b.WriteString(`{"package":"github.com/nooga/paserati/pkg/vm","name":"`)
		b.WriteString(name)
		b.WriteString(`","iterations":1000,"ns_per_op":`)
		b.WriteString(strconv.FormatFloat(ns, 'f', -1, 64))
		b.WriteString(`,"bytes_per_op":0,"allocs_per_op":0,"captured_at":"2026-07-12T00:00:00Z"}`)
		b.WriteByte('\n')
	}
	for _, v := range anchor {
		emit("BenchmarkRatchetAnchor", v)
	}
	for _, v := range family {
		emit("BenchmarkGetOwn/n=16/last", v)
	}
	return b.String()
}

func minOf(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

func meanOf(xs []float64) float64 {
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func approxEq(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= 1e-6*(1+b)
}
