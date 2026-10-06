package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// failureDetail is the one-line reason a record fails under metric.
func failureDetail(r *Record, metric Tier) string {
	if r.Status != StatusOK {
		return r.Detail
	}
	d := r.Tiers[metric].Detail
	if r.ParseCheck == "parse-only" {
		d += " [parse-only]"
	}
	return d
}

func category(r *Record, metric Tier) string {
	if r.Status != StatusOK {
		return r.Status
	}
	return r.Tiers[metric].Category
}

// printFailures prints one line per failing test, in test-ID order.
func printFailures(records []Record, metric Tier) {
	for i := range records {
		r := &records[i]
		if r.Status == StatusSkip || r.passes(metric) {
			continue
		}
		label := "FAIL"
		switch r.Status {
		case StatusTimeout:
			label = "TIMEOUT"
		case StatusCrash:
			label = "CRASH"
		}
		fmt.Printf("%s %d/%d [%s] %s - %s\n", label, i+1, len(records), category(r, metric), r.ID, failureDetail(r, metric))
	}
}

func printVerbose(records []Record, metric Tier) {
	for i := range records {
		r := &records[i]
		status := "PASS"
		switch {
		case r.Status == StatusSkip:
			status = "SKIP"
		case r.Status == StatusTimeout:
			status = "TIMEOUT"
		case r.Status == StatusCrash:
			status = "CRASH"
		case !r.passes(metric):
			status = "FAIL"
		}
		fmt.Printf("%-7s %d/%d [%s] %s", status, i+1, len(records), category(r, metric), r.ID)
		switch {
		case r.Status == StatusSkip:
			fmt.Printf(" - %s", r.SkipReason)
		case status != "PASS":
			if d := failureDetail(r, metric); d != "" {
				fmt.Printf(" - %s", d)
			}
		}
		fmt.Println()
	}
}

// printSummary prints the tier table and the supporting breakdowns.
func printSummary(records []Record, metric Tier, elapsed time.Duration) {
	var run, skipped, timeouts, crashes, panics, errs int
	skipReasons := map[string]int{}
	var expClean, expErrors, cleanFail int
	var parseCheck, parseOnly, unmatched int
	files := map[string]bool{}
	variantIDs := 0
	for i := range records {
		r := &records[i]
		files[r.Path] = true
		if len(r.Variant) > 0 {
			variantIDs++
		}
		if r.Status == StatusSkip {
			skipped++
			skipReasons[r.SkipReason]++
			continue
		}
		run++
		switch r.Status {
		case StatusTimeout:
			timeouts++
		case StatusCrash:
			crashes++
		case StatusPanic:
			panics++
		case StatusError:
			errs++
		}
		if r.expectClean() {
			expClean++
			if r.Status == StatusOK && len(r.Actual) > 0 {
				cleanFail++
			}
		} else {
			expErrors++
		}
		switch r.ParseCheck {
		case "parse+check":
			parseCheck++
		case "parse-only":
			parseOnly++
		}
		if r.BaselineUnmatched && len(r.Variant) > 0 {
			unmatched++
		}
	}

	fmt.Println()
	fmt.Println("=== TypeScript Conformance Test Results ===")
	fmt.Printf("Test IDs:   %d from %d files (%d IDs are variants of a multi-valued directive)\n", len(records), len(files), variantIDs)
	fmt.Printf("Run:        %d   (expect errors: %d, expect clean: %d)\n", run, expErrors, expClean)
	fmt.Printf("Skipped:    %d%s\n", skipped, formatCounts(skipReasons))
	fmt.Printf("Timeouts:   %d   Crashes: %d   Panics: %d   Harness errors: %d\n", timeouts, crashes, panics, errs)
	fmt.Printf("Parse errors: %d tests also type-checked (parse+check), %d parse-only (checker panicked on the partial AST)\n", parseCheck, parseOnly)
	if unmatched > 0 {
		fmt.Printf("Variants without a baseline although the test has baselines for other variants (treated as clean): %d\n", unmatched)
	}
	fmt.Printf("Clean fail (expected clean, we reported errors): %d\n", cleanFail)

	fmt.Println()
	fmt.Printf("%-18s %8s %8s %9s %9s\n", "Tier", "Pass", "Run", "Pass%run", "Pass%all")
	for _, t := range allTiers {
		pass := 0
		cleanPass, errMatch := 0, 0
		for i := range records {
			r := &records[i]
			if r.Status == StatusSkip || !r.passes(t) {
				continue
			}
			pass++
			if r.expectClean() {
				cleanPass++
			} else {
				errMatch++
			}
		}
		mark := " "
		if t == metric {
			mark = "*"
		}
		fmt.Printf("%-18s %8d %8d %8.1f%% %8.1f%% %s  (clean-pass %d, error-match %d)\n",
			t, pass, run, pct(pass, run), pct(pass, len(records)), mark, cleanPass, errMatch)
	}
	fmt.Printf("(* = -metric; Pass%%all counts skipped IDs in the denominator)\n")

	printUnsupported(records, metric)
	fmt.Printf("\nDuration: %v\n", elapsed.Round(time.Millisecond))
}

func formatCounts(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s %d", k, m[k])
	}
	return "  (" + strings.Join(parts, ", ") + ")"
}

// printUnsupported counts, per directive Paserati does not model, how many run
// tests use it and how many of those fail under metric.
func printUnsupported(records []Record, metric Tier) {
	type counts struct{ total, failing int }
	byOpt := map[string]*counts{}
	none := counts{}
	for i := range records {
		r := &records[i]
		if r.Status == StatusSkip {
			continue
		}
		seen := map[string]bool{}
		for _, u := range r.UnsupportedOptions {
			seen[optionKey(u)] = true
		}
		if len(seen) == 0 {
			none.total++
			if !r.passes(metric) {
				none.failing++
			}
		}
		for k := range seen {
			c := byOpt[k]
			if c == nil {
				c = &counts{}
				byOpt[k] = c
			}
			c.total++
			if !r.passes(metric) {
				c.failing++
			}
		}
	}
	keys := make([]string, 0, len(byOpt))
	for k := range byOpt {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := byOpt[keys[i]], byOpt[keys[j]]
		if a.failing != b.failing {
			return a.failing > b.failing
		}
		return keys[i] < keys[j]
	})
	fmt.Println()
	fmt.Printf("Unsupported options (directives Paserati does not model), failing under %s:\n", metric)
	fmt.Printf("  %-34s %8s %8s %8s\n", "option", "tests", "failing", "pass%")
	fmt.Printf("  %-34s %8d %8d %7.1f%%\n", "(none: fully modelled)", none.total, none.failing, pct(none.total-none.failing, none.total))
	for _, k := range keys {
		c := byOpt[k]
		fmt.Printf("  %-34s %8d %8d %7.1f%%\n", k, c.total, c.failing, pct(c.total-c.failing, c.total))
	}
}

// printSuiteSummary shows pass rates by directory. The GRAND TOTAL line format
// is parsed by tools/refresh_compliance_chart.py; do not change its columns.
// Crashes, panics and harness errors count under Fail.
func printSuiteSummary(records []Record, metric Tier) {
	type suiteStats struct{ total, passed, failed, skipped, timeouts int }
	suites := make(map[string]*suiteStats)
	for i := range records {
		r := &records[i]
		parts := strings.Split(filepath.Dir(r.Path), string(filepath.Separator))
		key := filepath.Dir(r.Path)
		if len(parts) >= 2 {
			key = filepath.Join(parts[0], parts[1])
		}
		s := suites[key]
		if s == nil {
			s = &suiteStats{}
			suites[key] = s
		}
		s.total++
		switch {
		case r.Status == StatusSkip:
			s.skipped++
		case r.Status == StatusTimeout:
			s.timeouts++
		case r.passes(metric):
			s.passed++
		default:
			s.failed++
		}
	}
	var names []string
	for name := range suites {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Printf("\n%-50s %8s %8s %8s %8s %8s %8s\n", "Suite", "Total", "Pass", "Fail", "Skip", "Timeout", "Pass%")
	fmt.Println(strings.Repeat("-", 110))
	var gt, gp, gf, gs, gto int
	for _, name := range names {
		s := suites[name]
		fmt.Printf("%-50s %8d %8d %8d %8d %8d %7.1f%%\n", name, s.total, s.passed, s.failed, s.skipped, s.timeouts, pct(s.passed, s.total))
		gt += s.total
		gp += s.passed
		gf += s.failed
		gs += s.skipped
		gto += s.timeouts
	}
	fmt.Println(strings.Repeat("-", 110))
	fmt.Printf("%-50s %8d %8d %8d %8d %8d %7.1f%%\n", "GRAND TOTAL", gt, gp, gf, gs, gto, pct(gp, gt))
}

// dumpHeader is the first line of every dump file.
func dumpHeader(metric Tier, tsVersion string) string {
	return fmt.Sprintf("# metric=%s ts=%s", metric, tsVersion)
}

// handleDumpMode saves results to a file: `+id` for passing, `-id` for failing
// tests; skipped tests are omitted.
func handleDumpMode(records []Record, dumpFile string, metric Tier, tsVersion string) {
	f, err := os.Create(dumpFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating dump file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	ids := make([]*Record, 0, len(records))
	for i := range records {
		if records[i].Status != StatusSkip {
			ids = append(ids, &records[i])
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].ID < ids[j].ID })
	fmt.Fprintln(f, dumpHeader(metric, tsVersion))
	passCount, failCount := 0, 0
	for _, r := range ids {
		if r.passes(metric) {
			fmt.Fprintf(f, "+%s\n", r.ID)
			passCount++
		} else {
			fmt.Fprintf(f, "-%s\n", r.ID)
			failCount++
		}
	}
	fmt.Printf("Dumped %d results to %s (%d passed, %d failed) [metric=%s]\n", passCount+failCount, dumpFile, passCount, failCount, metric)
}

// loadDump reads a dump file, returning its results and its header (if any).
func loadDump(path string) (results map[string]bool, header string, err error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	results = make(map[string]bool)
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# metric=") {
			header = line
			continue
		}
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case '+':
			results[line[1:]] = true
		case '-':
			results[line[1:]] = false
		}
	}
	return results, header, nil
}

// handleDiffMode compares current results against a dump file.
func handleDiffMode(records []Record, diffFile, dumpFile string, metric Tier, tsVersion string) {
	baseline, header, err := loadDump(diffFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading baseline: %v\n", err)
		os.Exit(1)
	}
	if want := dumpHeader(metric, tsVersion); header != want {
		if header == "" {
			fmt.Fprintf(os.Stderr, "WARNING: %s has no '# metric=' header (legacy dump, unknown metric); diffing against -metric %s may be meaningless\n", diffFile, metric)
		} else {
			fmt.Fprintf(os.Stderr, "WARNING: metric mismatch: %s is %q but this run is %q; the diff compares different definitions of pass\n", diffFile, header, want)
		}
	}

	current := make(map[string]bool)
	for i := range records {
		if records[i].Status != StatusSkip {
			current[records[i].ID] = records[i].passes(metric)
		}
	}
	var newPasses, newFailures []string
	onlyBaseline := 0
	for id, was := range baseline {
		now, exists := current[id]
		if !exists {
			onlyBaseline++
			continue
		}
		if !was && now {
			newPasses = append(newPasses, id)
		} else if was && !now {
			newFailures = append(newFailures, id)
		}
	}
	onlyCurrent := 0
	for id := range current {
		if _, ok := baseline[id]; !ok {
			onlyCurrent++
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
	fmt.Printf("Only in baseline: %d, only in current: %d\n", onlyBaseline, onlyCurrent)
	if dumpFile != "" {
		handleDumpMode(records, dumpFile, metric, tsVersion)
	}
}
