package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestMinVsMeanFlipsRegressionVerdict proves the point of the min reducer on
// real captured data: the reducer choice flips the actual pass/fail verdict,
// not just the reported number.
//
// Samples below were captured 2026-07-12 on an Apple M2 via
//
//	go test ./pkg/vm -bench 'BenchmarkGetOwn/n=16/last|BenchmarkRatchetAnchor' -count=15
//
// BenchmarkGetOwn/n=16/last is memory-bound (a deep own-property walk); under
// interference its runs pick up upward-only noise — here a clean cluster (~9.5)
// and a contention-slowed tail (~12–13.4). The register-only calibration anchor
// is far steadier. Feeding the SAME 15 samples through the two reducers:
//
//	min-reduced  ratio_to_anchor: 9.536 / 1.172 = 8.14   (true cost)  -> 0 regressions
//	mean-reduced ratio_to_anchor: 11.759 / 1.243 = 9.46  (+16%)       -> 1 regression
//
// against a baseline pinned at the true (min) cost and perf-pr's 10% budget. The
// mean reducer manufactures a phantom regression on code the run never touched;
// the min reducer does not. This is the verdict flip #22 removes.
func TestMinVsMeanFlipsRegressionVerdict(t *testing.T) {
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
	curMin, err := aggregateFromFile(path)
	if err != nil {
		t.Fatalf("aggregateFromFile: %v", err)
	}
	wantMinRatio := minOf(family) / minOf(anchor)
	if got := curMin.Benchmarks[famKey].RatioToAnchor; !approxEq(got, wantMinRatio) {
		t.Fatalf("aggregated ratio_to_anchor = %.4f, want %.4f (min-reduced)", got, wantMinRatio)
	}

	// 2. Accepted baseline = the true cost (min). Same machine so no fingerprint warning.
	baseline := Baseline{
		Machine:    curMin.Machine,
		Anchor:     AnchorRecord{NSPerOp: minOf(anchor)},
		Benchmarks: map[string]BenchmarkEntry{famKey: {RatioToAnchor: wantMinRatio}},
	}

	// 3. Same samples, mean reducer: the ratio inflates past budget.
	curMean := Baseline{
		Machine:    curMin.Machine,
		Anchor:     AnchorRecord{NSPerOp: meanOf(anchor)},
		Benchmarks: map[string]BenchmarkEntry{famKey: {RatioToAnchor: meanOf(family) / meanOf(anchor)}},
	}

	// 4. Drive the real regression check. min -> clean, mean -> phantom.
	if got := quietRegressions(baseline, curMin, budget); got != 0 {
		t.Errorf("min reducer: %d regression(s), want 0 — the samples ARE the accepted baseline", got)
	}
	if got := quietRegressions(baseline, curMean, budget); got != 1 {
		t.Errorf("mean reducer: %d regression(s), want 1 — the phantom #22 removes", got)
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
