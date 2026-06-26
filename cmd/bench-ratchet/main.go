// Command bench-ratchet runs Go benchmarks, normalizes results against
// a calibration anchor benchmark, and compares them to a stored
// baseline. The goal is to catch perf regressions in CI without
// requiring the CI machine to match the developer's machine: we report
// every benchmark as a multiple of BenchmarkRatchetAnchor's ns/op,
// which is a tight CPU loop with no allocations and no project code.
//
// # Architecture
//
// Two phases, each independently runnable:
//
//  1. capture — runs `go test -bench` per package, streams each
//     parsed benchmark line as one JSON object to a .jsonl file as
//     it arrives. Resilient: if a later benchmark hangs or panics,
//     earlier results are already on disk.
//  2. aggregate — reads one or more .jsonl files, normalizes against
//     the anchor, emits the consolidated baseline JSON.
//
// The check/update/show one-shot modes are wrappers: capture →
// aggregate → (compare | write | print).
//
// Modes:
//
//	bench-ratchet check     (default)   capture, aggregate, compare to baseline
//	bench-ratchet update                capture, aggregate, write baseline
//	bench-ratchet show                  capture, aggregate, print baseline
//	bench-ratchet capture               capture only, write .jsonl
//	bench-ratchet aggregate             aggregate prior .jsonl(s) into baseline
//	bench-ratchet snapshot              capture, aggregate, write immutable snapshot
//
// Flags:
//
//	-baseline string    baseline JSON path; with snapshot, snapshot output path
//	                    (default docs/perf/baseline.json)
//	-budget float       fractional regression tolerated (default 0.05)
//	-packages string    space-sep go packages to bench (default: discover)
//	-count int          go test -count (default 1)
//	-benchtime string   go test -benchtime (default 1s)
//	-filter string      regexp filter on bench names (default: all)
//	-timeout string     go test -timeout per package (default 10m)
//	-out string         capture .jsonl output path
//	                    (default docs/perf/.runs/<sha>-<ts>.jsonl)
//	-in string          aggregate .jsonl input path (default = -out path)
//
// The baseline records both absolute ns/op (useful when same-machine
// drift is what matters) and ratio_to_anchor (machine-portable). The
// check is anchor-normalized: a slowdown is flagged only when the
// CURRENT ratio_to_anchor exceeds the BASELINE ratio_to_anchor by more
// than -budget. If the CPU model differs from the baseline, we still
// compare (that's the whole point of the anchor), but we print a
// warning so a confused reader has context.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nooga/paserati/pkg/perfdata"
	"golang.org/x/perf/benchfmt"
)

const (
	defaultBaselinePath = "docs/perf/baseline.json"
	defaultRunsDir      = "docs/perf/.runs"
	defaultBudget       = 0.05
	defaultCount        = 1
	defaultBenchtime    = "1s"
	defaultTimeout      = "10m"
	anchorName          = "BenchmarkRatchetAnchor"
	anchorPackage       = "github.com/nooga/paserati/pkg/vm"
	schemaVersion       = 1
)

// defaultPackages is the scope when no -packages flag is given.
//
//   - pkg/vm: VM-runtime micro-benchmarks plus the calibration anchor.
//   - ./tests: the higher-level fib / matrix / prototype benchmarks that
//     exercise the full lexer→parser→checker→compiler→vm pipeline and
//     catch regressions the pkg/vm micro-benches don't see.
//
// The anchor lives in pkg/vm, so that package must always be in scope;
// aggregate fails loudly if the anchor benchmark isn't captured.
var defaultPackages = []string{
	"github.com/nooga/paserati/pkg/vm",
	"github.com/nooga/paserati/tests",
}

type Baseline = perfdata.Baseline
type Machine = perfdata.Machine
type AnchorRecord = perfdata.Anchor
type BenchmarkEntry = perfdata.BenchmarkEntry
type BenchmarkSample = perfdata.BenchmarkSample
type StreamRecord = perfdata.StreamRecord

// Result is the in-memory parse of one benchmark line.
type Result struct {
	Package     string
	Name        string
	Iterations  int64
	NSPerOp     float64
	BytesPerOp  int64
	AllocsPerOp int64
	Samples     []BenchmarkSample
}

// FullName is "<package>.<benchmark name without -N suffix>".
func (r Result) FullName() string {
	return r.Package + "." + r.Name
}

func main() {
	var (
		baselinePath    = flag.String("baseline", defaultBaselinePath, "baseline JSON path")
		budget          = flag.Float64("budget", defaultBudget, "fractional regression tolerated before flagging (0.05 = 5%)")
		packages        = flag.String("packages", "", "space-separated go packages to bench (default: discover)")
		count           = flag.Int("count", defaultCount, "go test -count")
		benchtime       = flag.String("benchtime", defaultBenchtime, "go test -benchtime")
		filter          = flag.String("filter", "", "regexp filter on benchmark names (default: all)")
		timeout         = flag.String("timeout", defaultTimeout, "go test -timeout per package")
		outPath         = flag.String("out", "", "capture .jsonl output path (default: docs/perf/.runs/<sha>-<ts>.jsonl)")
		inPath          = flag.String("in", "", "aggregate .jsonl input path (default: most recent under docs/perf/.runs/)")
		force           = flag.Bool("force", false, "with update: bypass the ratchet — write current numbers even where they'd loosen the bar. Use sparingly for accepted regressions.")
		shaOverride     = flag.String("sha", "", "override the SHA recorded for this run (default: git rev-parse HEAD of cwd). Use when aggregating a capture from a worktree that differs from cwd.")
		tags            = flag.String("tags", "", "go test -tags (default none)")
		format          = flag.String("format", "text", "report format: text (default, ANSI terminal), markdown (GitHub/Slack-friendly table), json (the raw baseline)")
		allowIncomplete = flag.Bool("allow-incomplete", false, "with check: tolerate package capture errors and missing baseline entries instead of failing (they shrink coverage, so the default is to fail)")
	)
	flag.Parse()

	mode := "check"
	if flag.NArg() > 0 {
		mode = flag.Arg(0)
	}
	switch mode {
	case "check", "update", "show", "capture", "aggregate", "snapshot":
	default:
		die("unknown mode %q (want check / update / show / capture / aggregate / snapshot)", mode)
	}

	// aggregate-only mode reads an existing .jsonl, no benchmarks run.
	if mode == "aggregate" || (mode == "snapshot" && *inPath != "") {
		path := *inPath
		if path == "" {
			var err error
			path, err = mostRecentRun()
			if err != nil {
				die("locate input: %v\n  hint: pass -in <file.jsonl>", err)
			}
		}
		fmt.Printf("bench-ratchet: aggregate from %s\n", path)
		current, err := aggregateFromFile(path)
		if err != nil {
			die("aggregate: %v", err)
		}
		if *shaOverride != "" {
			current.CapturedAtSHA = *shaOverride
		}
		if mode == "snapshot" {
			writeSnapshot(*baselinePath, current)
		} else {
			writeOrCheck(*baselinePath, current, "update", *budget, *force, *format, 0, *allowIncomplete)
		}
		return
	}

	// check / update / show / capture / snapshot all need a capture phase.
	manual := *packages != "" || *filter != ""
	filterRE, err := compileFilter(*filter)
	if err != nil {
		die("invalid -filter regexp: %v", err)
	}

	jobs, scope, err := buildJobs(*packages, *tags, manual, filterRE)
	if err != nil {
		die("%v", err)
	}

	if *outPath == "" {
		*outPath = defaultRunPath()
	}
	if dir := filepath.Dir(*outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}

	fmt.Printf("bench-ratchet: %s mode, %s, %d job(s), budget=%.1f%%\n",
		mode, scope, len(jobs), *budget*100)
	for _, j := range jobs {
		tagNote := ""
		if j.tags != "" {
			tagNote = " -tags " + j.tags
		}
		fmt.Printf("  - %s%s  (bench %s)\n", j.pkg, tagNote, j.filter.String())
	}
	fmt.Printf("  raw .jsonl: %s\n", *outPath)

	captureFailed, err := captureJobs(jobs, *count, *benchtime, *timeout, *outPath)
	if err != nil {
		die("capture: %v", err)
	}

	if mode == "capture" {
		fmt.Printf("\ncaptured → %s\n", *outPath)
		return
	}

	current, err := aggregateFromFile(*outPath)
	if err != nil {
		die("aggregate: %v", err)
	}
	if *shaOverride != "" {
		current.CapturedAtSHA = *shaOverride
	}

	if mode == "snapshot" {
		writeSnapshot(*baselinePath, current)
	} else {
		writeOrCheck(*baselinePath, current, mode, *budget, *force, *format, captureFailed, *allowIncomplete)
	}
}

func writeSnapshot(path string, current Baseline) {
	if path == defaultBaselinePath {
		die("snapshot mode needs -baseline <snapshot.json>; refusing to write %s", defaultBaselinePath)
	}
	if _, err := os.Stat(path); err == nil {
		die("snapshot %s already exists; refusing to overwrite immutable perf history", path)
	} else if err != nil && !os.IsNotExist(err) {
		die("stat snapshot %s: %v", path, err)
	}
	stampAll(&current)
	if err := writeBaseline(path, current); err != nil {
		die("write snapshot: %v", err)
	}
	fmt.Printf("\nwrote snapshot → %s (%d benchmarks)\n", path, len(current.Benchmarks))
}

// writeOrCheck dispatches the post-aggregate action.
func writeOrCheck(baselinePath string, current Baseline, mode string, budget float64, force bool, format string, captureFailed int, allowIncomplete bool) {
	switch mode {
	case "show":
		switch format {
		case "markdown":
			printBaselineMarkdown(current)
		case "json", "text", "":
			printBaseline(current)
		default:
			die("unknown -format %q (want text / markdown / json)", format)
		}
	case "update":
		// Ratchet semantics: if a baseline exists, MERGE rather than
		// overwrite. Each (benchmark, metric) only moves toward
		// faster / fewer-allocs / fewer-bytes. -force bypasses the
		// ratchet and writes current as-is.
		existing, err := readBaseline(baselinePath)
		if err != nil {
			// No prior baseline — first run. Stamp every entry with
			// current's commit/time and write.
			fmt.Printf("  no existing baseline at %s — writing current as the initial bar.\n", baselinePath)
			stampAll(&current)
			if err := writeBaseline(baselinePath, current); err != nil {
				die("write baseline: %v", err)
			}
			fmt.Printf("\nwrote baseline → %s (%d benchmarks)\n", baselinePath, len(current.Benchmarks))
			return
		}
		if force {
			fmt.Printf("  -force: writing current numbers as the new bar, bypassing ratchet.\n")
			stampAll(&current)
			if err := writeBaseline(baselinePath, current); err != nil {
				die("write baseline: %v", err)
			}
			fmt.Printf("\nwrote baseline → %s (%d benchmarks)\n", baselinePath, len(current.Benchmarks))
			return
		}
		merged, summary := ratchetMerge(existing, current)
		printRatchetSummary(summary)
		if err := writeBaseline(baselinePath, merged); err != nil {
			die("write baseline: %v", err)
		}
		fmt.Printf("\nwrote baseline → %s (%d benchmarks)\n", baselinePath, len(merged.Benchmarks))
	case "check":
		baseline, err := readBaseline(baselinePath)
		if err != nil {
			die("read baseline (%s): %v\n  hint: run `bench-ratchet update` to seed it",
				baselinePath, err)
		}
		res := compareAndReport(baseline, current, budget, format)
		fail := res.regressions > 0
		// A capture error or a missing baseline entry means we measured less
		// than the bar covers — fail unless explicitly tolerated, so the gate
		// can't pass green on shrunken coverage.
		if !allowIncomplete && (captureFailed > 0 || res.missing > 0) {
			fmt.Fprintf(os.Stderr,
				"\ncheck: failing on incomplete coverage — %d capture error(s), %d missing benchmark(s); pass -allow-incomplete to override\n",
				captureFailed, res.missing)
			fail = true
		}
		if fail {
			os.Exit(1)
		}
	}
}

// RatchetSummary describes what ratchetMerge changed. Each named bench
// falls into one of these buckets per metric. Each entry remembers the
// post-merge ns_per_op (the new bar's wall-clock per outer iteration)
// so the printed summary can include it as a derived "wall" column.
type RatchetSummary struct {
	Tightened []SummaryEntry // at least one metric moved toward "better"
	Pinned    []SummaryEntry // current was worse on at least one metric; baseline kept
	NewBench  []SummaryEntry // not in baseline; adopted as-is
	Missing   []SummaryEntry // in baseline but not in current; kept (rename guard)
}

// SummaryEntry is one line of the ratchet summary report.
type SummaryEntry struct {
	Name    string
	NSPerOp float64
}

// ratchetMerge produces a baseline whose per-benchmark metrics are the
// MIN of existing and current. Top-level fields (machine, anchor,
// captured_at, sha) are taken from current — those describe the run
// that produced this file, not the historical bar.
//
// Per-benchmark provenance (BestSinceSHA / BestSinceAt) tracks WHICH
// commit set the current bar for each benchmark:
//
//   - new benchmark (not in existing)        → stamp = current run
//   - tightened (any metric improved)        → stamp = current run
//   - all metrics ≥ baseline (no improvement) → stamp = unchanged
//   - missing from current                   → stamp = unchanged (entry kept)
//
// Returns a summary so the caller can print what moved.
func ratchetMerge(existing, current Baseline) (Baseline, RatchetSummary) {
	out := current // machine, anchor, captured_at, version, sha — all current
	out.Benchmarks = make(map[string]BenchmarkEntry, len(current.Benchmarks))
	var summary RatchetSummary

	for name, cur := range current.Benchmarks {
		base, ok := existing.Benchmarks[name]
		if !ok {
			cur.BestSinceSHA = current.CapturedAtSHA
			cur.BestSinceAt = current.CapturedAt
			out.Benchmarks[name] = cur
			summary.NewBench = append(summary.NewBench, SummaryEntry{Name: name, NSPerOp: cur.NSPerOp})
			continue
		}
		merged := BenchmarkEntry{
			NSPerOp:       minF(cur.NSPerOp, base.NSPerOp),
			AllocsPerOp:   minI(cur.AllocsPerOp, base.AllocsPerOp),
			BytesPerOp:    minI(cur.BytesPerOp, base.BytesPerOp),
			RatioToAnchor: minF(cur.RatioToAnchor, base.RatioToAnchor),
		}
		tightened := merged.RatioToAnchor < base.RatioToAnchor ||
			merged.AllocsPerOp < base.AllocsPerOp ||
			merged.BytesPerOp < base.BytesPerOp
		regressedOnSome := cur.RatioToAnchor > base.RatioToAnchor ||
			cur.AllocsPerOp > base.AllocsPerOp ||
			cur.BytesPerOp > base.BytesPerOp
		if tightened {
			merged.BestSinceSHA = current.CapturedAtSHA
			merged.BestSinceAt = current.CapturedAt
			merged.Samples = cur.Samples
			summary.Tightened = append(summary.Tightened, SummaryEntry{Name: name, NSPerOp: merged.NSPerOp})
		} else {
			merged.BestSinceSHA = base.BestSinceSHA
			merged.BestSinceAt = base.BestSinceAt
			merged.Samples = base.Samples
		}
		out.Benchmarks[name] = merged
		if regressedOnSome {
			summary.Pinned = append(summary.Pinned, SummaryEntry{Name: name, NSPerOp: merged.NSPerOp})
		}
	}

	// Baseline entries not in current — keep them, including their
	// provenance. A removed/renamed benchmark shouldn't release the
	// bar; use -force to drop intentionally.
	for name, base := range existing.Benchmarks {
		if _, ok := out.Benchmarks[name]; !ok {
			out.Benchmarks[name] = base
			summary.Missing = append(summary.Missing, SummaryEntry{Name: name, NSPerOp: base.NSPerOp})
		}
	}
	sortByName := func(a []SummaryEntry) { sort.Slice(a, func(i, j int) bool { return a[i].Name < a[j].Name }) }
	sortByName(summary.Tightened)
	sortByName(summary.Pinned)
	sortByName(summary.NewBench)
	sortByName(summary.Missing)
	return out, summary
}

func printRatchetSummary(s RatchetSummary) {
	fmt.Println()
	fmt.Printf("ratchet: %d tightened, %d would-have-regressed-but-pinned, %d new, %d missing-and-kept\n",
		len(s.Tightened), len(s.Pinned), len(s.NewBench), len(s.Missing))
	printGroup := func(prefix, header string, entries []SummaryEntry) {
		if len(entries) == 0 {
			return
		}
		fmt.Println("  " + header)
		for _, e := range entries {
			fmt.Printf("    %s %-80s  %10s\n", prefix, short(e.Name, 80), formatWall(e.NSPerOp))
		}
	}
	printGroup("+", "TIGHTENED (new bar adopted):", s.Tightened)
	printGroup("!", "PINNED (current worse on some metric; baseline kept — use check to see drift):", s.Pinned)
	printGroup("*", "NEW (adopted, no prior bar):", s.NewBench)
	printGroup("-", "MISSING (kept from baseline; use -force to drop if intentional):", s.Missing)
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func minI(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// stampAll fills in per-entry provenance from the baseline's top-level
// commit/time. Used for initial writes and -force writes where every
// entry is being "newly set" rather than ratcheted.
func stampAll(b *Baseline) {
	for name, e := range b.Benchmarks {
		e.BestSinceSHA = b.CapturedAtSHA
		e.BestSinceAt = b.CapturedAt
		b.Benchmarks[name] = e
	}
}

func defaultRunPath() string {
	ts := time.Now().UTC().Format("20060102T150405Z")
	sha := gitShortSHA()
	if sha == "" {
		sha = "nosha"
	}
	return filepath.Join(defaultRunsDir, fmt.Sprintf("%s-%s.jsonl", sha, ts))
}

// mostRecentRun returns the most recently-modified .jsonl under
// defaultRunsDir, or an error if none exist.
func mostRecentRun() (string, error) {
	ents, err := os.ReadDir(defaultRunsDir)
	if err != nil {
		return "", err
	}
	var best string
	var bestTime time.Time
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(bestTime) {
			best = filepath.Join(defaultRunsDir, e.Name())
			bestTime = info.ModTime()
		}
	}
	if best == "" {
		return "", fmt.Errorf("no .jsonl files in %s", defaultRunsDir)
	}
	return best, nil
}

func compileFilter(pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return regexp.MustCompile(`^Benchmark`), nil
	}
	return regexp.Compile(pattern)
}

// captureJob is one `go test -bench` invocation: a package under a specific
// build-tag set and benchmark filter.
type captureJob struct {
	pkg    string
	tags   string
	filter *regexp.Regexp
}

// buildJobs decides what gets benchmarked. With -packages or -filter it
// honors them verbatim (power-user escape hatch); otherwise it runs the
// default package scope with an all-benchmarks filter, which includes the
// calibration anchor in pkg/vm.
func buildJobs(packages, tags string, manual bool, filterRE *regexp.Regexp) ([]captureJob, string, error) {
	pkgList := strings.Fields(packages)
	if len(pkgList) == 0 {
		pkgList = append(pkgList, defaultPackages...)
	}
	jobs := make([]captureJob, 0, len(pkgList))
	for _, p := range pkgList {
		jobs = append(jobs, captureJob{pkg: p, tags: tags, filter: filterRE})
	}
	scope := "default scope"
	if manual {
		scope = "manual scope"
	}
	return jobs, scope, nil
}

// captureJobs returns the number of jobs whose `go test` invocation failed
// (build error, timeout, panic). A non-zero count means reduced benchmark
// coverage, which `check` treats as a failure unless -allow-incomplete is set.
func captureJobs(jobs []captureJob, count int, benchtime, timeout, outPath string) (int, error) {
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", outPath, err)
	}
	defer out.Close()
	enc := json.NewEncoder(out)

	failed := 0
	for i, j := range jobs {
		label := j.pkg
		if j.tags != "" {
			label += " -tags " + j.tags
		}
		fmt.Fprintf(os.Stderr, "  [%d/%d] %s ... ", i+1, len(jobs), label)
		n, err := captureOnePackage(j.pkg, count, benchtime, timeout, j.tags, j.filter, enc, out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: %v (%d records captured)\n", err, n)
			failed++
			continue
		}
		fmt.Fprintf(os.Stderr, "%d records\n", n)
	}
	return failed, nil
}

// captureOnePackage runs `go test -bench` on one package, reads its
// stdout incrementally via a pipe, parses each benchmark line, and
// writes a StreamRecord JSON line to enc/out per parsed line.
//
// Returns the number of records written. An error indicates the go
// test invocation itself failed (timeout, build error, missing
// package). Partial results before the failure are still flushed.
func captureOnePackage(pkg string, count int, benchtime, timeout, tags string, filter *regexp.Regexp, enc *json.Encoder, sync *os.File) (int, error) {
	args := []string{
		"test",
		"-run", "^$",
		"-bench", filter.String(),
		"-benchmem",
		"-count", strconv.Itoa(count),
		"-benchtime", benchtime,
		"-timeout", timeout,
	}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, pkg)
	cmd := exec.Command("go", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start: %w", err)
	}

	written := 0
	now := time.Now().UTC().Format(time.RFC3339)

	// benchfmt is the official Go-team parser for `go test -bench`
	// output (it's what benchstat uses internally). It tokenizes the
	// "ns/op", "B/op", "allocs/op" and any custom ReportMetric values
	// as a slice of (value, unit) pairs, sidestepping the regex
	// brittleness from having to know the column order — custom
	// metrics from b.ReportMetric land BETWEEN ns/op and B/op in
	// declaration order, which a hand-rolled regex can't anchor
	// against without listing every possible unit name.
	br := benchfmt.NewReader(stdout, pkg)
	for br.Scan() {
		rec, ok := br.Result().(*benchfmt.Result)
		if !ok {
			continue
		}
		// benchfmt strips the leading "Benchmark" from rec.Name.Full();
		// we re-prepend so the .jsonl + baseline see the standard form.
		// It also tags the result with a "-N" GOMAXPROCS suffix
		// (e.g. "RatchetAnchor-8"); strip via trimGoMaxProcsSuffix.
		name := "Benchmark" + trimGoMaxProcsSuffix(string(rec.Name.Full()))

		// benchfmt canonicalizes time units to "sec/op" so values are
		// comparable across benchmarks that emit "ns/op" vs "ms/op".
		// We store ns/op in the StreamRecord schema, so multiply back.
		stream := StreamRecord{
			Package:    pkg,
			Name:       name,
			Iterations: int64(rec.Iters),
			CapturedAt: now,
		}
		for _, v := range rec.Values {
			switch v.Unit {
			case "sec/op", "ns/op":
				if v.Unit == "sec/op" {
					stream.NSPerOp = v.Value * 1e9
				} else {
					stream.NSPerOp = v.Value
				}
			case "B/op":
				stream.BytesPerOp = int64(v.Value)
			case "allocs/op":
				stream.AllocsPerOp = int64(v.Value)
			}
		}
		if err := enc.Encode(stream); err != nil {
			return written, fmt.Errorf("encode: %w", err)
		}
		_ = sync.Sync() // make sure it survives a crash mid-sweep
		written++
	}
	if err := br.Err(); err != nil {
		_ = cmd.Wait()
		return written, fmt.Errorf("benchfmt scan: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return written, fmt.Errorf("go test: %w", err)
	}
	return written, nil
}

// aggregateFromFile reads a .jsonl of StreamRecord lines and returns
// a Baseline computed from them. Same-named records (e.g. multiple
// -count repetitions) are averaged.
func aggregateFromFile(path string) (Baseline, error) {
	f, err := os.Open(path)
	if err != nil {
		return Baseline{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	type accum struct {
		pkg                       string
		name                      string
		count                     int
		nsSum, bytesSum, allocSum float64
		iters                     int64
		samples                   []BenchmarkSample
	}
	byName := map[string]*accum{}
	dec := json.NewDecoder(f)
	for dec.More() {
		var rec StreamRecord
		if err := dec.Decode(&rec); err != nil {
			return Baseline{}, fmt.Errorf("decode: %w", err)
		}
		key := rec.Package + "." + rec.Name
		a := byName[key]
		if a == nil {
			a = &accum{pkg: rec.Package, name: rec.Name}
			byName[key] = a
		}
		a.count++
		a.nsSum += rec.NSPerOp
		a.bytesSum += float64(rec.BytesPerOp)
		a.allocSum += float64(rec.AllocsPerOp)
		if rec.Iterations > a.iters {
			a.iters = rec.Iterations
		}
		a.samples = append(a.samples, rec.Sample())
	}

	var results []Result
	for _, a := range byName {
		results = append(results, Result{
			Package:     a.pkg,
			Name:        a.name,
			Iterations:  a.iters,
			NSPerOp:     a.nsSum / float64(a.count),
			BytesPerOp:  int64(a.bytesSum / float64(a.count)),
			AllocsPerOp: int64(a.allocSum / float64(a.count)),
			Samples:     append([]BenchmarkSample(nil), a.samples...),
		})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].FullName() < results[j].FullName() })

	anchor, ok := findAnchor(results)
	if !ok {
		return Baseline{}, fmt.Errorf("anchor benchmark %q not found in %s", anchorName, path)
	}
	if anchor.NSPerOp <= 0 {
		return Baseline{}, fmt.Errorf("anchor ns/op is %.3f — divide-by-zero protection", anchor.NSPerOp)
	}
	return buildCurrentBaseline(results, anchor), nil
}

func findAnchor(results []Result) (Result, bool) {
	for _, r := range results {
		if r.Name == anchorName {
			return r, true
		}
	}
	return Result{}, false
}

func buildCurrentBaseline(results []Result, anchor Result) Baseline {
	m := detectMachine()
	bm := map[string]BenchmarkEntry{}
	for _, r := range results {
		if r.Name == anchorName {
			continue
		}
		samples := append([]BenchmarkSample(nil), r.Samples...)
		for i := range samples {
			samples[i].RatioToAnchor = samples[i].NSPerOp / anchor.NSPerOp
		}
		bm[r.FullName()] = BenchmarkEntry{
			NSPerOp:       r.NSPerOp,
			AllocsPerOp:   r.AllocsPerOp,
			BytesPerOp:    r.BytesPerOp,
			RatioToAnchor: r.NSPerOp / anchor.NSPerOp,
			Samples:       samples,
		}
	}
	return Baseline{
		Version:       schemaVersion,
		CapturedAt:    time.Now().UTC().Format(time.RFC3339),
		CapturedAtSHA: gitShortSHA(),
		Machine:       m,
		Anchor: AnchorRecord{
			Name:       anchor.Name,
			Package:    anchor.Package,
			NSPerOp:    anchor.NSPerOp,
			Iterations: anchor.Iterations,
			Samples:    append([]BenchmarkSample(nil), anchor.Samples...),
		},
		Benchmarks: bm,
	}
}

func detectMachine() Machine {
	return Machine{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		NumCPU:    runtime.NumCPU(),
		CPUModel:  detectCPUModel(),
		GoVersion: runtime.Version(),
	}
}

// detectCPUModel returns a human-readable CPU model string (e.g.
// "Apple M3", "Intel(R) Xeon(R) Platinum 8275CL CPU @ 3.00GHz") or
// "unknown" if probing fails. macOS uses sysctl; Linux reads
// /proc/cpuinfo; everything else returns the GOARCH.
func detectCPUModel() string {
	switch runtime.GOOS {
	case "darwin":
		if out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
			if s := strings.TrimSpace(string(out)); s != "" {
				return s
			}
		}
	case "linux":
		if body, err := os.ReadFile("/proc/cpuinfo"); err == nil {
			for _, line := range strings.Split(string(body), "\n") {
				if strings.HasPrefix(line, "model name") {
					if idx := strings.Index(line, ":"); idx >= 0 {
						return strings.TrimSpace(line[idx+1:])
					}
				}
			}
		}
	}
	return "unknown"
}

func gitShortSHA() string {
	out, err := exec.Command("git", "rev-parse", "--short=12", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func writeBaseline(path string, b Baseline) error {
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	body, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func readBaseline(path string) (Baseline, error) {
	var b Baseline
	body, err := os.ReadFile(path)
	if err != nil {
		return b, err
	}
	if err := json.Unmarshal(body, &b); err != nil {
		return b, err
	}
	if b.Version != schemaVersion {
		return b, fmt.Errorf("baseline version %d, want %d (regenerate with `update`)", b.Version, schemaVersion)
	}
	return b, nil
}

func printBaseline(b Baseline) {
	body, _ := json.MarshalIndent(b, "", "  ")
	fmt.Println(string(body))
}

// checkResult tallies what the compare found, so the caller can decide the
// exit code: regressions always fail; missing entries fail unless tolerated.
type checkResult struct{ regressions, missing, newCount int }

// compareAndReport prints a per-benchmark drift report and returns the tally.
// format is "text" (default ANSI terminal) or "markdown" (a single
// GitHub/Slack-friendly table).
func compareAndReport(baseline, current Baseline, budget float64, format string) checkResult {
	if format == "markdown" {
		return compareAndReportMarkdown(baseline, current, budget)
	}
	fmt.Println()
	if baseline.Machine.CPUModel != current.Machine.CPUModel ||
		baseline.Machine.GoVersion != current.Machine.GoVersion {
		fmt.Printf("warning: machine fingerprint differs from baseline\n")
		fmt.Printf("  baseline: %s / %s / %s\n",
			baseline.Machine.CPUModel, baseline.Machine.GoVersion, baseline.Machine.OS+"-"+baseline.Machine.Arch)
		fmt.Printf("  current:  %s / %s / %s\n",
			current.Machine.CPUModel, current.Machine.GoVersion, current.Machine.OS+"-"+current.Machine.Arch)
		fmt.Printf("  comparing on anchor-relative ratios — absolute ns/op deltas are not meaningful.\n\n")
	}
	if baseline.Anchor.NSPerOp > 0 {
		anchorDrift := (current.Anchor.NSPerOp - baseline.Anchor.NSPerOp) / baseline.Anchor.NSPerOp
		fmt.Printf("anchor: baseline %.3f ns/op, current %.3f ns/op (%+.1f%%)\n",
			baseline.Anchor.NSPerOp, current.Anchor.NSPerOp, anchorDrift*100)
	}
	fmt.Println()

	type drift struct {
		name                string
		baseRatio, curRatio float64
		baseNs, curNs       float64
		delta               float64
		present             bool
	}
	var drifts []drift
	for name, base := range baseline.Benchmarks {
		cur, ok := current.Benchmarks[name]
		d := drift{
			name:      name,
			baseRatio: base.RatioToAnchor,
			baseNs:    base.NSPerOp,
			present:   ok,
		}
		if ok {
			d.curRatio = cur.RatioToAnchor
			d.curNs = cur.NSPerOp
			d.delta = (cur.RatioToAnchor - base.RatioToAnchor) / base.RatioToAnchor
		}
		drifts = append(drifts, d)
	}
	sort.Slice(drifts, func(i, j int) bool {
		// Regressions first, biggest at top.
		return drifts[i].delta > drifts[j].delta
	})

	regressions := 0
	missing := 0
	// Header. `wall` is the wall-clock per outer iteration (= ns_per_op
	// formatted human-readable).
	fmt.Printf("%-60s  %12s  %12s  %8s  %10s\n",
		"benchmark", "baseline×", "current×", "Δ%", "wall")
	fmt.Println(strings.Repeat("-", 110))
	for _, d := range drifts {
		if !d.present {
			fmt.Printf("%-60s  %12.3f  %12s  %8s  %10s   MISSING\n",
				short(d.name, 60), d.baseRatio, "—", "—", formatWall(d.baseNs))
			missing++
			continue
		}
		mark := "  ok"
		if d.delta > budget {
			mark = "  REGRESSION"
			regressions++
		} else if d.delta < -budget {
			mark = "  IMPROVED"
		}
		fmt.Printf("%-60s  %12.3f  %12.3f  %+7.1f%%  %10s%s\n",
			short(d.name, 60), d.baseRatio, d.curRatio, d.delta*100, formatWall(d.curNs), mark)
	}
	// New benchmarks (in current, not in baseline)
	newCount := 0
	for name, cur := range current.Benchmarks {
		if _, ok := baseline.Benchmarks[name]; !ok {
			fmt.Printf("%-60s  %12s  %12.3f  %8s  %10s   NEW\n",
				short(name, 60), "—", cur.RatioToAnchor, "—", formatWall(cur.NSPerOp))
			newCount++
		}
	}

	fmt.Println()
	fmt.Printf("summary: %d regression(s) > %.1f%% budget, %d missing, %d new\n",
		regressions, budget*100, missing, newCount)
	return checkResult{regressions: regressions, missing: missing, newCount: newCount}
}

func short(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// trimGoMaxProcsSuffix removes the "-N" GOMAXPROCS suffix that go test
// appends to benchmark names ("BenchmarkX/sub-8"). Sub-benchmark
// paths use "/" so a trailing "-\d+" is unambiguously the cpus tag.
func trimGoMaxProcsSuffix(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '-' {
			suffix := name[i+1:]
			allDigit := suffix != ""
			for _, c := range suffix {
				if c < '0' || c > '9' {
					allDigit = false
					break
				}
			}
			if allDigit {
				return name[:i]
			}
			return name
		}
		if name[i] == '/' {
			return name
		}
	}
	return name
}

// formatWall renders a per-iteration time in human-readable units. The
// number is just ns_per_op — derived, not stored. Chosen unit minimizes
// digits while keeping at least 3 significant figures.
//
//	0.5 ns → "0.50 ns"   |  43.7 ns → "43.7 ns"   |  1234 ns → "1.23 µs"
//	1.5 ms → "1.50 ms"   |  688 ms → " 688 ms"    |  2.4 s → "2.40 s"
func formatWall(ns float64) string {
	switch {
	case ns < 1000:
		return fmt.Sprintf("%6.3g ns", ns)
	case ns < 1_000_000:
		return fmt.Sprintf("%6.3g µs", ns/1_000)
	case ns < 1_000_000_000:
		return fmt.Sprintf("%6.3g ms", ns/1_000_000)
	default:
		return fmt.Sprintf("%6.3g s", ns/1_000_000_000)
	}
}

// compareAndReportMarkdown is the markdown sibling of compareAndReport.
// Renders a single GitHub-Flavored-Markdown table with one row per
// benchmark. The "Status" column carries the per-row verdict (OK,
// IMPROVED, REGRESSION, NEW, MISSING). A summary line + machine
// fingerprint sit above the table.
//
// The output is also Slack-friendly when wrapped in a code block, since
// Slack renders the pipes monospace.
func compareAndReportMarkdown(baseline, current Baseline, budget float64) checkResult {
	type row struct {
		name          string
		baseR, curR   float64
		baseNs, curNs float64
		delta         float64
		bestSinceSHA  string
		status        string // "ok" / "REGRESSION" / "IMPROVED" / "NEW" / "MISSING"
	}
	var rows []row
	seen := map[string]bool{}
	regressions := 0
	missing := 0
	newCount := 0

	for name, base := range baseline.Benchmarks {
		seen[name] = true
		r := row{name: name, baseR: base.RatioToAnchor, baseNs: base.NSPerOp, bestSinceSHA: base.BestSinceSHA}
		cur, ok := current.Benchmarks[name]
		if !ok {
			r.status = "MISSING"
			missing++
			rows = append(rows, r)
			continue
		}
		r.curR = cur.RatioToAnchor
		r.curNs = cur.NSPerOp
		r.delta = (cur.RatioToAnchor - base.RatioToAnchor) / base.RatioToAnchor
		switch {
		case r.delta > budget:
			r.status = "REGRESSION"
			regressions++
		case r.delta < -budget:
			r.status = "IMPROVED"
		default:
			r.status = "ok"
		}
		rows = append(rows, r)
	}
	for name, cur := range current.Benchmarks {
		if seen[name] {
			continue
		}
		rows = append(rows, row{
			name:         name,
			curR:         cur.RatioToAnchor,
			curNs:        cur.NSPerOp,
			bestSinceSHA: cur.BestSinceSHA,
			status:       "NEW",
		})
		newCount++
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].delta > rows[j].delta })

	// Header preamble.
	fmt.Println()
	fmt.Printf("**bench-ratchet** — %d regression(s) > %.1f%% budget, %d missing, %d new\n\n",
		regressions, budget*100, missing, newCount)
	fmt.Printf("- baseline: `%s` / `%s` / `%s`\n",
		baseline.Machine.CPUModel, baseline.Machine.GoVersion, baseline.Machine.OS+"-"+baseline.Machine.Arch)
	fmt.Printf("- current:  `%s` / `%s` / `%s`\n",
		current.Machine.CPUModel, current.Machine.GoVersion, current.Machine.OS+"-"+current.Machine.Arch)
	if baseline.Anchor.NSPerOp > 0 {
		anchorDrift := (current.Anchor.NSPerOp - baseline.Anchor.NSPerOp) / baseline.Anchor.NSPerOp
		fmt.Printf("- anchor: baseline `%.3f ns/op`, current `%.3f ns/op` (`%+.1f%%`)\n\n",
			baseline.Anchor.NSPerOp, current.Anchor.NSPerOp, anchorDrift*100)
	} else {
		fmt.Println()
	}

	// Table.
	fmt.Println("| Benchmark | Baseline× | Current× | Δ% | Wall | Best since | Status |")
	fmt.Println("|---|---:|---:|---:|---:|---|---|")
	for _, r := range rows {
		switch r.status {
		case "MISSING":
			fmt.Printf("| `%s` | %.3f | — | — | %s | `%s` | %s |\n",
				mdEscape(r.name), r.baseR, formatWallMD(r.baseNs), shortSHA(r.bestSinceSHA), r.status)
		case "NEW":
			fmt.Printf("| `%s` | — | %.3f | — | %s | `%s` | %s |\n",
				mdEscape(r.name), r.curR, formatWallMD(r.curNs), shortSHA(r.bestSinceSHA), r.status)
		default:
			fmt.Printf("| `%s` | %.3f | %.3f | %+.1f%% | %s | `%s` | %s |\n",
				mdEscape(r.name), r.baseR, r.curR, r.delta*100, formatWallMD(r.curNs), shortSHA(r.bestSinceSHA), r.status)
		}
	}

	return checkResult{regressions: regressions, missing: missing, newCount: newCount}
}

// printBaselineMarkdown emits the entire baseline as a GFM table — one
// row per benchmark with the current bar, the commit that set it, and
// the human-readable wall time. Sorted by ratio_to_anchor descending so
// the heaviest benchmarks land at top.
func printBaselineMarkdown(b Baseline) {
	fmt.Printf("**bench-ratchet baseline** — captured %s @ `%s`\n\n",
		b.CapturedAt, b.CapturedAtSHA)
	fmt.Printf("- machine: `%s` / `%s` / `%s`\n", b.Machine.CPUModel, b.Machine.GoVersion, b.Machine.OS+"-"+b.Machine.Arch)
	fmt.Printf("- anchor: `%s` at `%.3f ns/op`\n\n", b.Anchor.Name, b.Anchor.NSPerOp)

	type row struct {
		name  string
		entry BenchmarkEntry
	}
	rows := make([]row, 0, len(b.Benchmarks))
	for name, e := range b.Benchmarks {
		rows = append(rows, row{name, e})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].entry.RatioToAnchor > rows[j].entry.RatioToAnchor })

	fmt.Println("| Benchmark | Anchor× | Wall | Allocs/op | Bytes/op | Best since |")
	fmt.Println("|---|---:|---:|---:|---:|---|")
	for _, r := range rows {
		fmt.Printf("| `%s` | %.3f | %s | %d | %d | `%s` |\n",
			mdEscape(r.name), r.entry.RatioToAnchor, formatWallMD(r.entry.NSPerOp),
			r.entry.AllocsPerOp, r.entry.BytesPerOp, shortSHA(r.entry.BestSinceSHA))
	}
}

// mdEscape escapes characters that would otherwise be interpreted by
// the Markdown table cell parser. The only meaningful one for benchmark
// names is `|`, which would split the row; Go doesn't allow `|` in
// identifiers but we encode benchmarks as `pkg.name` so we still defend
// against future surprises.
func mdEscape(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

// shortSHA renders a SHA for the markdown "Best since" column.
// Returns "—" for empty inputs (e.g., baselines that pre-date the
// provenance field).
func shortSHA(s string) string {
	if s == "" {
		return "—"
	}
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// formatWallMD is formatWall without the leading-space padding that
// makes terminal columns line up — markdown right-aligns numerically
// via the `|---:|` header, so the spaces just add visual noise.
func formatWallMD(ns float64) string {
	switch {
	case ns < 1000:
		return fmt.Sprintf("%.3g ns", ns)
	case ns < 1_000_000:
		return fmt.Sprintf("%.3g µs", ns/1_000)
	case ns < 1_000_000_000:
		return fmt.Sprintf("%.3g ms", ns/1_000_000)
	default:
		return fmt.Sprintf("%.3g s", ns/1_000_000_000)
	}
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "bench-ratchet: "+format+"\n", args...)
	os.Exit(2)
}
