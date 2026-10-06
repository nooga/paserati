// paserati-testtsc runs TypeScript's own conformance tests through Paserati's
// parser and type checker and compares the diagnostics with TypeScript's
// reference baselines (tests/baselines/reference/*.errors.txt).
//
// Every test is scored under five tiers (see score.go); -metric picks the one
// that decides pass/fail for -dump/-diff/-suite. Tests run in worker
// subprocesses (pool.go) so a hang or a fatal stack overflow in one test costs
// that test, not the run.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/nooga/paserati/pkg/parser"
)

// testCase is one (file, variant) pair after discovery, directive stripping and
// baseline lookup.
type testCase struct {
	ID           string
	Rel          string
	Variant      Variant
	Skip         string // non-empty: not run, reason
	Unsupported  []string
	Expected     Baseline
	BaselineFile string
	Unmatched    bool // baselines exist for the test name, but none for this variant
	Job          int  // index into the job list, -1 when skipped
}

// Record is the per-test result written by -json and used for every report.
type Record struct {
	ID                 string              `json:"id"`
	Path               string              `json:"path"`
	Variant            map[string]string   `json:"variant,omitempty"`
	Status             string              `json:"status"` // ok | skip | timeout | crash | panic | error
	SkipReason         string              `json:"skipReason,omitempty"`
	Detail             string              `json:"detail,omitempty"`
	BaselineFile       string              `json:"baselineFile,omitempty"`
	BaselineUnmatched  bool                `json:"baselineUnmatched,omitempty"`
	Expected           []Diag              `json:"expected"`
	ExpectedOtherFile  []Diag              `json:"expectedOtherFile,omitempty"`
	ExpectedGlobal     []Diag              `json:"expectedGlobal,omitempty"`
	Actual             []Diag              `json:"actual"`
	ParseErrors        int                 `json:"parseErrors"`
	ParseCheck         string              `json:"parseCheck,omitempty"` // "" (no parse errors) | parse+check | parse-only
	Tiers              map[Tier]TierResult `json:"tiers,omitempty"`
	UnsupportedOptions []string            `json:"unsupportedOptions,omitempty"`
	DurationMs         float64             `json:"durationMs"`
}

func (r *Record) passes(t Tier) bool {
	if r.Status != StatusOK {
		return false
	}
	return r.Tiers[t].Pass
}

func (r *Record) expectClean() bool { return len(r.Expected) == 0 }

func main() {
	var (
		tscPath     = flag.String("path", "", "Path to TypeScript repository root")
		subPath     = flag.String("subpath", "", "Subdirectory within tests/cases/conformance/ (e.g., 'expressions', 'types/primitives')")
		timeout     = flag.Duration("timeout", 2*time.Second, "Hard wall-clock deadline per test (the worker is killed when it passes)")
		verbose     = flag.Bool("verbose", false, "Show individual test results")
		limit       = flag.Int("limit", 0, "Limit number of test files (0 = all)")
		suiteMode   = flag.Bool("suite", false, "Show pass rates by directory")
		_           = flag.Bool("single-only", true, "Deprecated no-op: multi-file tests are always skipped (and counted)")
		dumpFile    = flag.String("dump", "", "Dump results to file (+id/-id format, header records the metric)")
		diffFile    = flag.String("diff", "", "Compare against a dump file")
		pattern     = flag.String("pattern", "*.ts,*.tsx", "Comma-separated file patterns")
		skipPattern = flag.String("skip", "", "Skip files matching this substring")
		skipFile    = flag.String("skipfile", "", "File containing test paths to skip (one per line, relative to conformance dir)")
		strictCheck = flag.Bool("strict-errors", false, "Alias for -metric codes-superset")
		metricFlag  = flag.String("metric", "", "Tier that decides pass/fail for -dump/-diff/-suite: loose|codes-superset|codes-exact|line-code-exact|full-exact (default loose)")
		jsonFile    = flag.String("json", "", "Write per-test records (expected/actual diagnostics, tiers, options) to this file")
		jobs        = flag.Int("j", max(1, runtime.NumCPU()/2), "Number of parallel worker processes")
		maxSize     = flag.Int("max-size", 0, "Skip test files larger than this many bytes (0 = no limit)")
		worker      = flag.Bool("worker", false, "internal: run as a worker process")
		workerDir   = flag.String("conformance-dir", "", "internal: conformance dir for -worker")
	)
	flag.Parse()
	parser.DumpASTEnabled = false

	if *worker {
		workerMain(*workerDir)
		return
	}

	if *tscPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: paserati-testtsc -path /path/to/TypeScript\n")
		fmt.Fprintf(os.Stderr, "\nRuns TypeScript conformance tests against Paserati's type checker.\n")
		fmt.Fprintf(os.Stderr, "Compares results against TypeScript's baseline files.\n")
		os.Exit(1)
	}

	metric := TierLoose
	if *strictCheck {
		metric = TierCodesSuperset
	}
	if *metricFlag != "" {
		t, ok := parseTier(*metricFlag)
		if !ok {
			fmt.Fprintf(os.Stderr, "Error: unknown -metric %q (want one of %v)\n", *metricFlag, allTiers)
			os.Exit(1)
		}
		metric = t
	}

	conformanceDir := filepath.Join(*tscPath, "tests", "cases", "conformance")
	baselinesDir := filepath.Join(*tscPath, "tests", "baselines", "reference")
	if _, err := os.Stat(conformanceDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: conformance tests not found at %s\n", conformanceDir)
		os.Exit(1)
	}
	index, err := buildBaselineIndex(baselinesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: baselines not found at %s: %v\n", baselinesDir, err)
		os.Exit(1)
	}
	tsVersion := readTSVersion(*tscPath)

	searchDir := conformanceDir
	if *subPath != "" {
		searchDir = filepath.Join(conformanceDir, *subPath)
	}
	files, err := findTestFiles(searchDir, *pattern, *skipPattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding test files: %v\n", err)
		os.Exit(1)
	}
	skipList := loadSkipList(*skipFile)

	cases := prepareCases(files, conformanceDir, index, skipList, *limit, *maxSize)
	var reqs []workerReq
	for i := range cases {
		if cases[i].Skip == "" {
			cases[i].Job = len(reqs)
			reqs = append(reqs, workerReq{Rel: cases[i].Rel, Params: cases[i].Variant})
		}
	}
	fmt.Printf("Running %d test IDs (%d files discovered, %d skipped) from: %s  [metric=%s, -j %d, deadline %v]\n",
		len(reqs), len(files), len(cases)-len(reqs), searchDir, metric, *jobs, *timeout)

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot locate own executable: %v\n", err)
		os.Exit(1)
	}
	start := time.Now()
	tty := stderrIsTerminal()
	outcomes := runPool(exe, conformanceDir, reqs, *jobs, *timeout, func(done int) {
		if tty && (done%25 == 0 || done == len(reqs)) {
			fmt.Fprintf(os.Stderr, "\r\033[K%d/%d", done, len(reqs))
		}
	})
	if tty {
		fmt.Fprintf(os.Stderr, "\r\033[K")
	}
	elapsed := time.Since(start)

	records := make([]Record, len(cases))
	for i, tc := range cases {
		var oc jobOutcome
		if tc.Job >= 0 {
			oc = outcomes[tc.Job]
		}
		records[i] = buildRecord(tc, oc)
	}

	if *jsonFile != "" {
		if err := writeJSON(*jsonFile, records); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", *jsonFile, err)
			os.Exit(1)
		}
	}

	switch {
	case *diffFile != "":
		handleDiffMode(records, *diffFile, *dumpFile, metric, tsVersion)
	case *dumpFile != "":
		handleDumpMode(records, *dumpFile, metric, tsVersion)
	default:
		if *suiteMode {
			printSuiteSummary(records, metric)
		} else if !*verbose {
			printFailures(records, metric)
		}
	}
	if *verbose {
		printVerbose(records, metric)
	}
	printSummary(records, metric, elapsed)
}

func stderrIsTerminal() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func readTSVersion(tscPath string) string {
	content, err := os.ReadFile(filepath.Join(tscPath, "package.json"))
	if err != nil {
		return "unknown"
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(content, &pkg) != nil || pkg.Version == "" {
		return "unknown"
	}
	return pkg.Version
}

// findTestFiles discovers test files under searchDir. pattern is a
// comma-separated list of globs matched against the base name.
func findTestFiles(searchDir, pattern, skipPattern string) ([]string, error) {
	var globs []string
	for _, g := range strings.Split(pattern, ",") {
		if g = strings.TrimSpace(g); g != "" {
			globs = append(globs, g)
		}
	}
	var files []string
	err := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		matched := false
		for _, g := range globs {
			ok, err := filepath.Match(g, filepath.Base(path))
			if err != nil {
				return err
			}
			if ok {
				matched = true
				break
			}
		}
		if !matched || strings.HasSuffix(path, ".d.ts") {
			return nil
		}
		if skipPattern != "" && strings.Contains(path, skipPattern) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	sort.Strings(files)
	return files, err
}

// prepareCases reads every file, strips directives, expands variants and looks
// up baselines. Files that cannot be run (multi-file, tsx, too large) yield a
// single skipped case so they stay visible in the denominators.
func prepareCases(files []string, conformanceDir string, index *baselineIndex, skipList map[string]bool, limit, maxSize int) []testCase {
	var cases []testCase
	runnable := 0
	for _, f := range files {
		rel, _ := filepath.Rel(conformanceDir, f)
		if skipList[rel] {
			continue
		}
		skipCase := func(reason string) testCase {
			return testCase{ID: rel, Rel: rel, Skip: reason, Job: -1}
		}
		if strings.HasSuffix(f, ".tsx") {
			cases = append(cases, skipCase("tsx-unsupported"))
			continue
		}
		content, err := os.ReadFile(f)
		if err != nil {
			cases = append(cases, skipCase("read-error"))
			continue
		}
		if maxSize > 0 && len(content) > maxSize {
			cases = append(cases, skipCase("too-large"))
			continue
		}
		source := strings.TrimPrefix(string(content), "\xEF\xBB\xBF")
		st := stripDirectives(source)
		if st.HasFilename {
			cases = append(cases, skipCase("multi-file"))
			continue
		}
		if limit > 0 && runnable >= limit {
			break
		}
		runnable++

		base := filepath.Base(f)
		name := strings.TrimSuffix(base, filepath.Ext(base))
		variants, tooMany := expandVariants(st.Options)
		for _, v := range variants {
			id := rel + v.Suffix()
			if skipList[id] {
				continue
			}
			tc := testCase{ID: id, Rel: rel, Variant: v, Job: -1}
			if len(v) == 0 {
				tc.Variant = nil
			}
			if tooMany {
				tc.Skip = "too-many-variants"
				cases = append(cases, tc)
				continue
			}
			file, unmatched := index.lookup(name, v)
			tc.BaselineFile, tc.Unmatched = file, unmatched
			bl, err := index.load(file, base)
			if err != nil {
				tc.Skip = "baseline-read-error"
				cases = append(cases, tc)
				continue
			}
			tc.Expected = bl
			tc.Unsupported = unsupportedOptionsFor(effectiveOptions(st.Options, v))
			cases = append(cases, tc)
		}
	}
	return cases
}

// buildRecord turns a case and its outcome into a scored record.
func buildRecord(tc testCase, oc jobOutcome) Record {
	rec := Record{
		ID: tc.ID, Path: tc.Rel, Variant: tc.Variant, BaselineFile: tc.BaselineFile,
		BaselineUnmatched: tc.Unmatched, Expected: tc.Expected.Diags,
		ExpectedOtherFile: tc.Expected.Other, ExpectedGlobal: tc.Expected.Global,
		UnsupportedOptions: tc.Unsupported, Actual: []Diag{},
	}
	if rec.Expected == nil {
		rec.Expected = []Diag{}
	}
	if tc.Skip != "" {
		rec.Status, rec.SkipReason = StatusSkip, tc.Skip
		return rec
	}
	rec.Status, rec.Detail = oc.Status, oc.Detail
	rec.DurationMs = float64(oc.Resp.DurNs) / 1e6
	if oc.Status != StatusOK {
		return rec
	}
	if oc.Resp.Actual != nil {
		rec.Actual = oc.Resp.Actual
	}
	rec.ParseErrors = oc.Resp.ParseErrors
	switch {
	case oc.Resp.ParseErrors == 0:
	case oc.Resp.CheckRan:
		rec.ParseCheck = "parse+check"
	default:
		rec.ParseCheck = "parse-only"
		rec.Detail = "checker panicked after parse errors: " + oc.Resp.CheckPanic
	}
	// tsc reports diagnostics in position order; the checker's own order varies
	// between runs (map iteration), so normalise it before anything else sees it.
	sort.SliceStable(rec.Actual, func(i, j int) bool {
		a, b := rec.Actual[i], rec.Actual[j]
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Col != b.Col {
			return a.Col < b.Col
		}
		return a.Code < b.Code
	})
	rec.Tiers = scoreAll(rec.Expected, rec.Actual)
	return rec
}

func writeJSON(path string, records []Record) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	// One record per line keeps the file greppable and diffable.
	if _, err := f.WriteString("[\n"); err != nil {
		return err
	}
	for i, r := range records {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		sep := ","
		if i == len(records)-1 {
			sep = ""
		}
		if _, err := fmt.Fprintf(f, "%s%s\n", b, sep); err != nil {
			return err
		}
	}
	_, err = f.WriteString("]\n")
	return err
}

// loadSkipList reads a file of test paths to skip (one per line, relative to the
// conformance dir). Lines starting with # are comments.
func loadSkipList(path string) map[string]bool {
	if path == "" {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read skipfile %s: %v\n", path, err)
		return nil
	}
	result := make(map[string]bool)
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		result[line] = true
	}
	return result
}

func pct(num, denom int) float64 {
	if denom == 0 {
		return 0
	}
	return float64(num) * 100 / float64(denom)
}
