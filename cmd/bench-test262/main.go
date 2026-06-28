// Command bench-test262 turns a `paserati-test262 -json` run into perf
// StreamRecord(s) that bench-ratchet's aggregate step normalizes against
// the calibration anchor — so the Test262 macro-benchmark lands on the
// timeline page as just another series, with no new normalization code.
//
// The metric is the SUM of per-test execution time over the tests that
// passed and did NOT time out. Two deliberate properties:
//
//   - Summing per-test durations (not wall-clock) is parallelism-invariant:
//     the runner can fan out for throughput without moving the number.
//   - Restricting to the passing, non-timed-out set decouples speed from
//     correctness: a regression that makes tests time out changes the
//     timeout COUNT (reported separately), not the timing sum.
//
// Usage:
//
//	paserati-test262 -path ./test262 -subpath language -json | bench-test262 >> run.jsonl
//	bench-test262 -in results.json -out run.jsonl
//
// Emits one record for the whole suite (test262/total) plus one per
// top-level suite (test262/<suite>) for the page's Breakdown toggle.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nooga/paserati/pkg/perfdata"
	"github.com/nooga/paserati/pkg/test262"
)

func main() {
	var (
		inPath    = flag.String("in", "", "paserati-test262 -json input (default: stdin)")
		outPath   = flag.String("out", "", "append StreamRecord JSONL here (default: stdout)")
		timestamp = flag.String("timestamp", "", "RFC3339 capture time (default: now, UTC)")
	)
	flag.Parse()

	out := readResults(*inPath)

	capturedAt := strings.TrimSpace(*timestamp)
	if capturedAt == "" {
		capturedAt = time.Now().UTC().Format(time.RFC3339)
	}

	records := buildRecords(out, capturedAt)
	writeRecords(records, *outPath)
}

// buildRecords sums per-test durations over the passing, non-timed-out set,
// emitting a total plus per-top-level-suite breakdown. NSPerOp carries the
// summed nanoseconds; aggregate divides it by the anchor to normalize.
func buildRecords(out test262.Output, capturedAt string) []perfdata.StreamRecord {
	type acc struct {
		sumNS float64
		count int64
	}
	bySuite := map[string]*acc{}
	total := &acc{}

	for _, r := range out.Results {
		if !r.Passed || r.TimedOut {
			continue
		}
		ns := float64(r.Duration) // time.Duration marshals as int64 nanoseconds
		total.sumNS += ns
		total.count++

		suite := topLevelSuite(r.Path)
		a := bySuite[suite]
		if a == nil {
			a = &acc{}
			bySuite[suite] = a
		}
		a.sumNS += ns
		a.count++
	}

	rec := func(name string, a *acc) perfdata.StreamRecord {
		return perfdata.StreamRecord{
			Package:    "test262",
			Name:       name,
			Iterations: a.count, // tests contributing to the sum
			NSPerOp:    a.sumNS, // summed execution time; anchor-normalized downstream
			CapturedAt: capturedAt,
		}
	}

	records := []perfdata.StreamRecord{rec("total", total)}
	suites := make([]string, 0, len(bySuite))
	for s := range bySuite {
		suites = append(suites, s)
	}
	sort.Strings(suites)
	for _, s := range suites {
		records = append(records, rec(s, bySuite[s]))
	}
	return records
}

// topLevelSuite extracts the chapter from a path like
// "test262/test/built-ins/Math/abs/x.js" -> "built-ins".
func topLevelSuite(path string) string {
	const marker = "/test/"
	i := strings.Index(path, marker)
	if i < 0 {
		return "unknown"
	}
	rest := path[i+len(marker):]
	if j := strings.IndexByte(rest, '/'); j >= 0 {
		return rest[:j]
	}
	return "unknown"
}

func readResults(inPath string) test262.Output {
	f := os.Stdin
	if inPath != "" {
		var err error
		f, err = os.Open(inPath)
		if err != nil {
			die("open -in: %v", err)
		}
		defer f.Close()
	}
	var out test262.Output
	if err := json.NewDecoder(f).Decode(&out); err != nil {
		die("decode test262 json: %v", err)
	}
	if len(out.Results) == 0 {
		die("no results in input (did the run produce -json output?)")
	}
	return out
}

func writeRecords(records []perfdata.StreamRecord, outPath string) {
	w := os.Stdout
	if outPath != "" {
		f, err := os.OpenFile(outPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			die("open -out: %v", err)
		}
		defer f.Close()
		w = f
	}
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	enc := json.NewEncoder(bw)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			die("encode record: %v", err)
		}
	}
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "bench-test262: "+format+"\n", args...)
	os.Exit(1)
}
