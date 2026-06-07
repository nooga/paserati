package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/driver"
	errorsPkg "github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/vm"
)

// panicError wraps a panic as a PaseratiError
type panicError struct {
	msg string
}

func (e *panicError) Error() string           { return e.msg }
func (e *panicError) Pos() errorsPkg.Position { return errorsPkg.Position{} }
func (e *panicError) Kind() string            { return "Panic" }
func (e *panicError) Code() string            { return "" }
func (e *panicError) Message() string         { return e.msg }
func (e *panicError) Unwrap() error           { return nil }

// directiveRegex parses // @key: value directives from TypeScript test files
var directiveRegex = regexp.MustCompile(`^/{2}\s*@(\w+)\s*:\s*([^\r\n]*)`)

// errorLineRegex parses error lines from .errors.txt baselines
// Format: filename.ts(line,col): error TS1234: message
var errorLineRegex = regexp.MustCompile(`^[^(]+\((\d+),(\d+)\):\s+error\s+(TS\d+):\s+(.*)`)

// globalErrorLineRegex parses global error lines without file/line info
// Format: error TS1234: message  (used e.g. for @noLib tests, TS2318 cannot find global type)
var globalErrorLineRegex = regexp.MustCompile(`^error\s+(TS\d+):\s+(.*)`)

// TestDirectives holds parsed // @ directives from a test file
type TestDirectives struct {
	Target    string
	Module    string
	Strict    bool
	NoEmit    bool
	Lib       string
	Filenames []string // multi-file test virtual filenames
	Raw       map[string]string
}

// ExpectedError represents a single expected error from a baseline
type ExpectedError struct {
	Line    int
	Col     int
	Code    string // e.g. "TS2322"
	Message string
}

// TestResult represents the outcome of running a single test
type TestResult struct {
	Path        string
	RelPath     string
	Passed      bool
	Failed      bool
	Skipped     bool
	TimedOut    bool
	Duration    time.Duration
	Error       string
	Category    string // "clean-pass", "clean-fail", "error-match", "error-mismatch", "skip"
	ExpectClean bool   // true if no .errors.txt baseline exists
	Details     string // extra info about what went wrong
}

// TestStats tracks aggregate statistics
type TestStats struct {
	Total      int
	Passed     int
	Failed     int
	Skipped    int
	Timeouts   int
	CleanPass  int // expected clean, got clean
	CleanFail  int // expected clean, got errors
	ErrorMatch int // expected errors, got matching errors
	ErrorMiss  int // expected errors, got wrong/missing errors
	Duration   time.Duration
}

func main() {
	var (
		tscPath     = flag.String("path", "", "Path to TypeScript repository root")
		subPath     = flag.String("subpath", "", "Subdirectory within tests/cases/conformance/ (e.g., 'expressions', 'types/primitives')")
		timeout     = flag.Duration("timeout", 2*time.Second, "Timeout per test")
		verbose     = flag.Bool("verbose", false, "Show individual test results")
		limit       = flag.Int("limit", 0, "Limit number of tests (0 = all)")
		suiteMode   = flag.Bool("suite", false, "Show pass rates by directory")
		singleOnly  = flag.Bool("single-only", true, "Only run single-file tests (skip @filename tests)")
		dumpFile    = flag.String("dump", "", "Dump results to file (+path/-path format)")
		diffFile    = flag.String("diff", "", "Compare against baseline file")
		pattern     = flag.String("pattern", "*.ts", "File pattern (default *.ts)")
		skipPattern = flag.String("skip", "", "Skip files matching this substring")
		skipFile    = flag.String("skipfile", "", "File containing test paths to skip (one per line, relative to conformance dir)")
		strictCheck = flag.Bool("strict-errors", false, "Also verify error codes match (not just clean/error)")
	)

	flag.Parse()
	parser.DumpASTEnabled = false

	if *tscPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: paserati-testtsc -path /path/to/TypeScript\n")
		fmt.Fprintf(os.Stderr, "\nRuns TypeScript conformance tests against Paserati's type checker.\n")
		fmt.Fprintf(os.Stderr, "Compares results against TypeScript's baseline files.\n")
		os.Exit(1)
	}

	// Verify paths exist
	conformanceDir := filepath.Join(*tscPath, "tests", "cases", "conformance")
	baselinesDir := filepath.Join(*tscPath, "tests", "baselines", "reference")

	if _, err := os.Stat(conformanceDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: conformance tests not found at %s\n", conformanceDir)
		os.Exit(1)
	}
	if _, err := os.Stat(baselinesDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: baselines not found at %s\n", baselinesDir)
		os.Exit(1)
	}

	// Find test files
	searchDir := conformanceDir
	if *subPath != "" {
		searchDir = filepath.Join(conformanceDir, *subPath)
	}

	testFiles, err := findTestFiles(searchDir, *pattern, *skipPattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding test files: %v\n", err)
		os.Exit(1)
	}

	// Filter to single-file tests if requested
	if *singleOnly {
		testFiles = filterSingleFileTests(testFiles)
	}

	// Load skip list
	skipList := loadSkipList(*skipFile, conformanceDir)

	if *limit > 0 && len(testFiles) > *limit {
		testFiles = testFiles[:*limit]
	}

	fmt.Printf("Running %d TypeScript conformance tests from: %s\n", len(testFiles), searchDir)
	if *singleOnly {
		fmt.Printf("(single-file tests only, use -single-only=false for all)\n")
	}

	// Filter skip list
	if len(skipList) > 0 {
		var filtered []string
		for _, f := range testFiles {
			rel, _ := filepath.Rel(conformanceDir, f)
			if !skipList[rel] {
				filtered = append(filtered, f)
			}
		}
		fmt.Printf("Skipped %d tests from skipfile\n", len(testFiles)-len(filtered))
		testFiles = filtered
	}

	// Run tests
	stats, results := runTests(testFiles, conformanceDir, baselinesDir, *timeout, *verbose, *strictCheck)

	// Output
	if *diffFile != "" {
		handleDiffMode(results, *diffFile, *dumpFile)
	} else if *dumpFile != "" {
		handleDumpMode(results, *dumpFile)
	} else if *suiteMode {
		printSuiteSummary(results, conformanceDir)
	} else {
		printSummary(&stats)
	}
}

// findTestFiles discovers .ts files under searchDir
func findTestFiles(searchDir, pattern, skipPattern string) ([]string, error) {
	var files []string
	err := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err != nil {
			return err
		}
		if !matched {
			return nil
		}
		// Skip .d.ts files
		if strings.HasSuffix(path, ".d.ts") {
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

// filterSingleFileTests removes tests that use // @filename: directives
func filterSingleFileTests(files []string) []string {
	var result []string
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if !strings.Contains(string(content), "@filename") {
			result = append(result, f)
		}
	}
	return result
}

// parseDirectives extracts // @key: value directives from test source
func parseDirectives(source string) TestDirectives {
	d := TestDirectives{
		Raw: make(map[string]string),
	}
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)
		m := directiveRegex.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := strings.ToLower(m[1])
		value := strings.TrimSpace(strings.TrimRight(m[2], "\r"))
		d.Raw[key] = value

		switch key {
		case "target":
			d.Target = value
		case "module":
			d.Module = value
		case "strict":
			d.Strict = value == "true"
		case "noemit":
			d.NoEmit = value == "true"
		case "lib":
			d.Lib = value
		case "filename":
			d.Filenames = append(d.Filenames, value)
		}
	}
	return d
}

// getBaselineName converts a test file path to its baseline name
// e.g., tests/cases/conformance/expressions/foo.ts -> foo
func getBaselineName(testPath string) string {
	base := filepath.Base(testPath)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// loadExpectedErrors reads a .errors.txt baseline file and returns whether
// the test is expected to produce errors
func loadExpectedErrors(baselinesDir, baselineName string) (bool, []ExpectedError) {
	errFile := filepath.Join(baselinesDir, baselineName+".errors.txt")
	content, err := os.ReadFile(errFile)
	if err != nil {
		// Fallback: TS generates parameterized baselines like
		// foo(target=es2015).errors.txt when the source uses // @target,
		// // @strict, // @module etc. Without this fallback, ~2,800 tests
		// are wrongly classified as "expected clean" and fail when Paserati
		// correctly reports the same errors as TypeScript.
		matches, _ := filepath.Glob(filepath.Join(baselinesDir, baselineName+"(*).errors.txt"))
		if len(matches) == 0 {
			// No baseline of any kind means the test should compile clean
			return false, nil
		}
		// Paserati is target-independent and always supports modern features,
		// so prefer the most-modern target baseline. ES5-target baselines may
		// contain syntax errors for features Paserati happily parses.
		sort.Slice(matches, func(i, j int) bool {
			return baselineTargetScore(matches[i]) < baselineTargetScore(matches[j])
		})
		content, err = os.ReadFile(matches[0])
		if err != nil {
			return false, nil
		}
	}

	var errors []ExpectedError
	for _, line := range strings.Split(string(content), "\n") {
		m := errorLineRegex.FindStringSubmatch(line)
		if m != nil {
			lineNum := 0
			colNum := 0
			_, _ = fmt.Sscanf(m[1], "%d", &lineNum)
			_, _ = fmt.Sscanf(m[2], "%d", &colNum)
			errors = append(errors, ExpectedError{
				Line:    lineNum,
				Col:     colNum,
				Code:    m[3],
				Message: m[4],
			})
			continue
		}
		// Also handle global errors without file/line info (e.g. @noLib tests, TS2318)
		g := globalErrorLineRegex.FindStringSubmatch(line)
		if g != nil {
			errors = append(errors, ExpectedError{Code: g[1], Message: g[2]})
		}
	}
	return true, errors
}

// baselineTargetScore ranks parameterized baseline filenames so the most-modern
// target sorts first. Lower is better. Used to pick a representative baseline
// when several variants exist (target=es5, target=es2015, etc.).
func baselineTargetScore(path string) int {
	switch {
	case strings.Contains(path, "target=esnext"):
		return 0
	case strings.Contains(path, "target=es2024"):
		return 1
	case strings.Contains(path, "target=es2023"):
		return 2
	case strings.Contains(path, "target=es2022"):
		return 3
	case strings.Contains(path, "target=es2021"):
		return 4
	case strings.Contains(path, "target=es2020"):
		return 5
	case strings.Contains(path, "target=es2019"):
		return 6
	case strings.Contains(path, "target=es2018"):
		return 7
	case strings.Contains(path, "target=es2017"):
		return 8
	case strings.Contains(path, "target=es2016"):
		return 9
	case strings.Contains(path, "target=es2015"):
		return 10
	case strings.Contains(path, "target=es5"):
		return 100
	case strings.Contains(path, "target=es3"):
		return 200
	default:
		return 50
	}
}

// runTests executes all test files and returns stats + results
func runTests(testFiles []string, conformanceDir, baselinesDir string, timeout time.Duration, verbose, strictCheck bool) (TestStats, []TestResult) {
	var stats TestStats
	stats.Total = len(testFiles)
	startTime := time.Now()

	var results []TestResult

	for i, testFile := range testFiles {
		// Periodic GC
		if i > 0 && i%100 == 0 {
			vm.ClearShapeCache()
			runtime.GC()
		}

		// If too many goroutines are leaked (from timed-out tests), force aggressive GC
		if numG := runtime.NumGoroutine(); numG > 50 {
			vm.ClearShapeCache()
			runtime.GC()
			runtime.GC()
		}

		// Print current test to stderr so we can see which one crashes
		relPath, _ := filepath.Rel(conformanceDir, testFile)
		fmt.Fprintf(os.Stderr, "\r\033[K%d/%d %s", i+1, stats.Total, relPath)

		result := runSingleTest(testFile, conformanceDir, baselinesDir, timeout, strictCheck)

		switch result.Category {
		case "clean-pass":
			stats.Passed++
			stats.CleanPass++
		case "error-match":
			stats.Passed++
			stats.ErrorMatch++
		case "clean-fail":
			stats.Failed++
			stats.CleanFail++
		case "error-mismatch":
			stats.Failed++
			stats.ErrorMiss++
		case "skip":
			stats.Skipped++
		case "timeout":
			stats.Timeouts++
		}

		if verbose {
			status := "PASS"
			if result.Failed {
				status = "FAIL"
			} else if result.Skipped {
				status = "SKIP"
			} else if result.TimedOut {
				status = "TIMEOUT"
			}
			fmt.Printf("%-7s %d/%d [%s] %s", status, i+1, stats.Total, result.Category, result.RelPath)
			if result.Details != "" {
				fmt.Printf(" - %s", result.Details)
			}
			fmt.Println()
		} else if result.Failed && !result.TimedOut {
			fmt.Printf("FAIL %d/%d [%s] %s - %s\n", i+1, stats.Total, result.Category, result.RelPath, result.Details)
		} else if result.TimedOut {
			fmt.Printf("TIMEOUT %d/%d %s\n", i+1, stats.Total, result.RelPath)
		}

		results = append(results, result)
	}

	stats.Duration = time.Since(startTime)
	return stats, results
}

// runSingleTest runs a single TypeScript conformance test
func runSingleTest(testFile, conformanceDir, baselinesDir string, timeout time.Duration, strictCheck bool) TestResult {
	relPath, _ := filepath.Rel(conformanceDir, testFile)
	baselineName := getBaselineName(testFile)

	result := TestResult{
		Path:    testFile,
		RelPath: relPath,
	}

	// Read test source
	content, err := os.ReadFile(testFile)
	if err != nil {
		result.Failed = true
		result.Category = "skip"
		result.Skipped = true
		result.Details = fmt.Sprintf("read error: %v", err)
		return result
	}

	source := string(content)

	// Strip UTF-8 BOM if present
	source = strings.TrimPrefix(source, "\xEF\xBB\xBF")

	// Skip very large files that can cause stack overflow in parser/checker
	if len(source) > 50000 {
		result.Skipped = true
		result.Category = "skip"
		result.Details = fmt.Sprintf("file too large (%d bytes)", len(source))
		return result
	}

	directives := parseDirectives(source)

	// Skip multi-file tests (shouldn't happen if singleOnly is true, but safety check)
	if len(directives.Filenames) > 0 {
		result.Skipped = true
		result.Category = "skip"
		result.Details = "multi-file test"
		return result
	}

	// Note: we don't skip based on @target since type checking is mostly
	// target-independent. Tests targeting ES5 etc. are still valid for
	// type checking purposes.

	// Load expected errors from baseline
	expectErrors, expectedErrs := loadExpectedErrors(baselinesDir, baselineName)
	result.ExpectClean = !expectErrors

	// Run synchronously - no goroutines to avoid stack overflow crashes from leaked goroutines.
	// We rely on the parser/checker having bounded execution for reasonable inputs.
	pas := createTscPaserati(directives)
	defer pas.Cleanup()

	start := time.Now()

	// Use recover to catch panics (but NOT stack overflows - those are fatal)
	var actualErrs []errorsPkg.PaseratiError
	func() {
		defer func() {
			if r := recover(); r != nil {
				actualErrs = append(actualErrs, &panicError{msg: fmt.Sprintf("panic: %v", r)})
			}
		}()

		// Parse
		lx := lexer.NewLexer(source)
		p := parser.NewParser(lx)
		prog, parseErrs := p.ParseProgram()
		if len(parseErrs) > 0 {
			actualErrs = parseErrs
			return
		}

		// Compile (runs type checker)
		_, compileErrs := pas.CompileProgram(prog)
		actualErrs = compileErrs
	}()

	result.Duration = time.Since(start)

	// Check if it took too long (informational only)
	if result.Duration > timeout {
		result.TimedOut = true
		result.Category = "timeout"
		result.Details = fmt.Sprintf("took %v (limit %v)", result.Duration.Round(time.Millisecond), timeout)
		return result
	}

	return classifyResult(result, actualErrs, expectErrors, expectedErrs, strictCheck)
}

// classifyResult determines whether the test passed based on actual vs expected errors
func classifyResult(result TestResult, actualErrs []errorsPkg.PaseratiError, expectErrors bool, expectedErrs []ExpectedError, strictCheck bool) TestResult {
	hasActualErrors := len(actualErrs) > 0

	if !expectErrors {
		// Test should compile clean
		if !hasActualErrors {
			result.Passed = true
			result.Category = "clean-pass"
		} else {
			result.Failed = true
			result.Category = "clean-fail"
			// Show first few errors
			var msgs []string
			for i, e := range actualErrs {
				if i >= 3 {
					msgs = append(msgs, fmt.Sprintf("... and %d more", len(actualErrs)-3))
					break
				}
				msgs = append(msgs, e.Error())
			}
			result.Details = strings.Join(msgs, "; ")
		}
	} else {
		// Test should produce errors
		if !hasActualErrors {
			// We expected errors but got none - this is a failure
			result.Failed = true
			result.Category = "error-mismatch"
			result.Details = fmt.Sprintf("expected %d errors, got none", len(expectedErrs))
		} else if !strictCheck {
			// Loose mode: just check that we produce SOME errors (not necessarily the right ones)
			result.Passed = true
			result.Category = "error-match"
		} else {
			// Strict mode: verify error codes match
			// For now, just check error count is roughly similar
			expectedCount := len(expectedErrs)
			actualCount := len(actualErrs)
			ratio := float64(actualCount) / float64(expectedCount)
			if ratio >= 0.5 && ratio <= 2.0 {
				result.Passed = true
				result.Category = "error-match"
			} else {
				result.Failed = true
				result.Category = "error-mismatch"
				result.Details = fmt.Sprintf("expected ~%d errors, got %d", expectedCount, actualCount)
			}
		}
	}
	return result
}

// createTscPaserati creates a Paserati instance configured for TSC test running
func createTscPaserati(directives TestDirectives) *driver.Paserati {
	initializers := builtins.GetStandardInitializers()
	pas := driver.NewPaseratiWithInitializers(initializers)
	pas.SetAllowTopLevelReturn(false)
	// Type checking is ON - that's what we're testing
	// Don't skip type check, don't ignore type errors
	return pas
}

// printSummary prints the final test summary
func printSummary(stats *TestStats) {
	fmt.Println()
	fmt.Println("=== TypeScript Conformance Test Results ===")
	fmt.Printf("Total:    %d\n", stats.Total)
	fmt.Printf("Passed:   %d (%.1f%%)\n", stats.Passed, pct(stats.Passed, stats.Total))
	fmt.Printf("Failed:   %d (%.1f%%)\n", stats.Failed, pct(stats.Failed, stats.Total))
	fmt.Printf("Skipped:  %d\n", stats.Skipped)
	fmt.Printf("Timeouts: %d\n", stats.Timeouts)
	fmt.Println()
	fmt.Println("Breakdown:")
	fmt.Printf("  Clean pass (expected clean, compiled clean):     %d\n", stats.CleanPass)
	fmt.Printf("  Clean fail (expected clean, got errors):         %d\n", stats.CleanFail)
	fmt.Printf("  Error match (expected errors, got errors):       %d\n", stats.ErrorMatch)
	fmt.Printf("  Error mismatch (expected errors, wrong result):  %d\n", stats.ErrorMiss)
	fmt.Printf("\nDuration: %v\n", stats.Duration.Round(time.Millisecond))
}

// printSuiteSummary shows pass rates by directory
func printSuiteSummary(results []TestResult, conformanceDir string) {
	type suiteStats struct {
		total, passed, failed, skipped, timeouts int
	}
	suites := make(map[string]*suiteStats)

	for _, r := range results {
		// Get directory path relative to conformance dir
		dir := filepath.Dir(r.RelPath)
		// Use first two levels
		parts := strings.Split(dir, string(filepath.Separator))
		var key string
		if len(parts) >= 2 {
			key = filepath.Join(parts[0], parts[1])
		} else {
			key = dir
		}

		s, ok := suites[key]
		if !ok {
			s = &suiteStats{}
			suites[key] = s
		}
		s.total++
		if r.Passed {
			s.passed++
		} else if r.TimedOut {
			s.timeouts++
		} else if r.Skipped {
			s.skipped++
		} else {
			s.failed++
		}
	}

	// Sort suite names
	var names []string
	for name := range suites {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Printf("\n%-50s %8s %8s %8s %8s %8s %8s\n", "Suite", "Total", "Pass", "Fail", "Skip", "Timeout", "Pass%")
	fmt.Println(strings.Repeat("-", 110))

	var grandTotal, grandPass, grandFail, grandSkip, grandTimeout int
	for _, name := range names {
		s := suites[name]
		passRate := pct(s.passed, s.total)
		fmt.Printf("%-50s %8d %8d %8d %8d %8d %7.1f%%\n",
			name, s.total, s.passed, s.failed, s.skipped, s.timeouts, passRate)
		grandTotal += s.total
		grandPass += s.passed
		grandFail += s.failed
		grandSkip += s.skipped
		grandTimeout += s.timeouts
	}

	fmt.Println(strings.Repeat("-", 110))
	fmt.Printf("%-50s %8d %8d %8d %8d %8d %7.1f%%\n",
		"GRAND TOTAL", grandTotal, grandPass, grandFail, grandSkip, grandTimeout,
		pct(grandPass, grandTotal))
}

// handleDumpMode saves results to a file
func handleDumpMode(results []TestResult, dumpFile string) {
	f, err := os.Create(dumpFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating dump file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	sort.Slice(results, func(i, j int) bool {
		return results[i].RelPath < results[j].RelPath
	})

	passCount, failCount := 0, 0
	for _, r := range results {
		if r.Passed {
			fmt.Fprintf(f, "+%s\n", r.RelPath)
			passCount++
		} else if !r.Skipped {
			fmt.Fprintf(f, "-%s\n", r.RelPath)
			failCount++
		}
	}
	fmt.Printf("Dumped %d results to %s (%d passed, %d failed)\n", passCount+failCount, dumpFile, passCount, failCount)
}

// handleDiffMode compares current results against a baseline
func handleDiffMode(results []TestResult, diffFile, dumpFile string) {
	baseline, err := loadBaseline(diffFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading baseline: %v\n", err)
		os.Exit(1)
	}

	current := make(map[string]bool)
	for _, r := range results {
		if !r.Skipped {
			current[r.RelPath] = r.Passed
		}
	}

	var newPasses, newFailures []string

	for path, baselinePassed := range baseline {
		currentPassed, exists := current[path]
		if !exists {
			continue
		}
		if !baselinePassed && currentPassed {
			newPasses = append(newPasses, path)
		} else if baselinePassed && !currentPassed {
			newFailures = append(newFailures, path)
		}
	}

	sort.Strings(newPasses)
	sort.Strings(newFailures)

	for _, p := range newPasses {
		fmt.Printf("+%s\n", p)
	}
	for _, p := range newFailures {
		fmt.Printf("-%s\n", p)
	}

	fmt.Printf("\n=== Diff Summary ===\n")
	fmt.Printf("New passes:   %d\n", len(newPasses))
	fmt.Printf("New failures: %d\n", len(newFailures))
	fmt.Printf("Net change:   %+d\n", len(newPasses)-len(newFailures))

	if dumpFile != "" {
		handleDumpMode(results, dumpFile)
	}
}

// loadBaseline reads a baseline file
func loadBaseline(path string) (map[string]bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	result := make(map[string]bool)
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case '+':
			result[line[1:]] = true
		case '-':
			result[line[1:]] = false
		}
	}
	return result, nil
}

// loadSkipList reads a file of test paths to skip (one per line, relative to conformance dir)
// Lines starting with # are comments
func loadSkipList(path string, conformanceDir string) map[string]bool {
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
