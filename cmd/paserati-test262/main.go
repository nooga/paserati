package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	errorsPkg "github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/test262"
	"github.com/nooga/paserati/pkg/vm"
)

func main() {
	// Parse command line flags
	var (
		testPath    = flag.String("path", "", "Path to test262 directory")
		pattern     = flag.String("pattern", "*.js", "File pattern for test files")
		subPath     = flag.String("subpath", "", "Subdirectory pattern within test/ (e.g., 'language/**', 'built-ins/Array/**')")
		skipPattern = flag.String("skip", "", "Skip files matching this pattern (e.g., 'Temporal', '**/Temporal/**')")
		verbose     = flag.Bool("verbose", false, "Verbose output")
		limit       = flag.Int("limit", 0, "Limit number of tests to run (0 = no limit)")
		timeout     = flag.Duration("timeout", 5*time.Second, "Timeout per test (e.g., 5s, 1m)")
		memprofile  = flag.String("memprofile", "", "Write memory profile to file")
		cpuprofile  = flag.String("cpuprofile", "", "Write CPU profile to file")
		gcstats     = flag.Bool("gcstats", false, "Print garbage collection statistics")
		treeMode    = flag.Bool("tree", false, "Show results as directory tree with aggregated stats")
		suiteMode   = flag.Bool("suite", false, "Show pass rates for each test suite (annexB, built-ins, intl402, language, staging)")
		disasm      = flag.Bool("disasm", false, "Print bytecode disassembly on failures")
		dumpFile    = flag.String("dump", "", "Dump all test results to file (format: +test-path or -test-path)")
		diffFile    = flag.String("diff", "", "Compare current results against baseline file and show differences")
		strictOnly  = flag.Bool("strict-only", false, "Skip tests with 'noStrict' flag (only run strict mode tests)")
		jsonFlag    = flag.Bool("json", false, "Output results in JSON format")
		allowFile   = flag.String("allow-failures", "", "Allow-list baseline file: exit 0 iff current failures are a subset of the failures listed there (paths prefixed '-')")
	)

	flag.Parse()
	// Ensure AST dump is off for harness runs unless explicitly enabled
	parser.DumpASTEnabled = false

	// CPU profiling
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	if *testPath == "" {
		fmt.Fprintf(os.Stderr, "Error: test262 path not specified\n")
		fmt.Fprintf(os.Stderr, "Usage: %s -path /path/to/test262\n", os.Args[0])
		os.Exit(1)
	}

	// Verify test262 directory exists
	testDir := filepath.Join(*testPath, "test")
	if _, err := os.Stat(testDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: test262 test directory not found at %s\n", testDir)
		os.Exit(1)
	}

	if !*jsonFlag {
		fmt.Printf("Running Test262 suite from: %s\n", *testPath)
	}

	// Find test files
	searchDir := testDir
	if *subPath != "" {
		searchDir = filepath.Join(testDir, *subPath)
		// Remove ** from the end if present for directory search
		searchDir = strings.TrimSuffix(searchDir, "/**")
	}

	testFiles, err := findTestFiles(searchDir, *pattern, *subPath, *skipPattern, *jsonFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding test files: %v\n", err)
		os.Exit(1)
	}

	if *limit > 0 && len(testFiles) > *limit {
		testFiles = testFiles[:*limit]
	}

	if !*jsonFlag {
		fmt.Printf("Found %d test files\n", len(testFiles))
	}

	// In -json mode, swap os.Stdout to stderr during test execution so that
	// anything a test prints via console.log (which lands on os.Stdout via the
	// console builtin) can't pollute the JSON stream paserati-analyze decodes.
	// Restored before the final json.Encoder write.
	realStdout := os.Stdout
	if *jsonFlag {
		os.Stdout = os.Stderr
	}

	// Run tests
	opts := runOptions{
		testRoot:   *testPath,
		testDir:    testDir,
		timeout:    *timeout,
		strictOnly: *strictOnly,
		verbose:    *verbose,
		jsonMode:   *jsonFlag,
	}
	stats, fileResults := runTests(testFiles, opts, *treeMode, *suiteMode, *disasm)
	corpus := corpusRevision(*testPath)

	if *jsonFlag {
		os.Stdout = realStdout
	}

	// Handle diff/dump modes
	if *diffFile != "" {
		handleDiffMode(fileResults, testDir, *diffFile, *dumpFile, corpus)
	} else if *dumpFile != "" {
		handleDumpMode(fileResults, testDir, *dumpFile, corpus)
	} else if *jsonFlag {
		// JSON output mode — stdout is now guaranteed clean.
		output := test262.Output{Policy: test262.Policy, Corpus: corpus, Stats: stats, Results: fileResults}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(output); err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Print summary, tree, or suite
		if *suiteMode {
			printSuiteSummary(fileResults, testDir, testPath)
		} else if *treeMode {
			printTreeSummary(fileResults, testDir)
		} else {
			printSummary(&stats)
		}
	}

	// Memory profiling and GC stats
	if *memprofile != "" {
		runtime.GC() // Force GC before profiling
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		defer f.Close()
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
		fmt.Printf("Memory profile written to %s\n", *memprofile)
	}

	if *gcstats {
		printGCStats()
	}

	// Stop CPU profile before exiting (os.Exit skips defers)
	if *cpuprofile != "" {
		pprof.StopCPUProfile()
	}

	// Exit code: -allow-failures, when set, overrides the default
	// "any failure is fatal" behavior with subset-of-baseline semantics.
	if *allowFile != "" {
		os.Exit(allowFailuresExitCode(fileResults, testDir, *allowFile))
	}
	if stats.Failed > 0 || stats.Timeouts > 0 || stats.InfraErrors > 0 {
		os.Exit(1)
	}
}

// corpusRevision is the checked-out test262 commit, part of a result's
// identity ("unknown" when it can't be read).
func corpusRevision(testPath string) string {
	out, err := exec.Command("git", "-C", testPath, "rev-parse", "--short=12", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// dumpHeader starts every -dump file: results from a different policy or
// corpus are not comparable line for line.
func dumpHeader(corpus string) string {
	return fmt.Sprintf("# paserati-test262 policy=%d corpus=%s", test262.Policy, corpus)
}

// allowFailuresExitCode loads a baseline file (same `+path` / `-path` format
// produced by -dump) and returns 0 if every currently-failing test appears
// as an allowed failure (`-path`) in the baseline. New failures cause exit 1;
// new passes are reported on stderr as info but don't change the exit code.
func allowFailuresExitCode(results []test262.Result, testDir, baselinePath string) int {
	data, err := os.ReadFile(baselinePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "allow-failures: cannot read %s: %v\n", baselinePath, err)
		return 2
	}
	allowed := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		if len(line) < 2 || line[0] != '-' {
			continue
		}
		allowed[strings.TrimSpace(line[1:])] = true
	}

	var newFailures, newPasses []string
	for _, r := range results {
		relPath, _ := filepath.Rel(testDir, r.Path)
		if relPath == "" {
			relPath = r.Path
		}
		switch {
		case (r.Failed || r.TimedOut) && !allowed[relPath]:
			newFailures = append(newFailures, relPath)
		case r.Passed && allowed[relPath]:
			newPasses = append(newPasses, relPath)
		}
	}

	if len(newPasses) > 0 {
		fmt.Fprintf(os.Stderr, "allow-failures: %d test(s) newly passing (consider bumping baseline):\n", len(newPasses))
		for _, p := range newPasses {
			fmt.Fprintf(os.Stderr, "  + %s\n", p)
		}
	}
	if len(newFailures) > 0 {
		fmt.Fprintf(os.Stderr, "allow-failures: %d new failure(s) not in %s:\n", len(newFailures), baselinePath)
		for _, p := range newFailures {
			fmt.Fprintf(os.Stderr, "  - %s\n", p)
		}
		return 1
	}
	return 0
}

// TreeNode represents a directory in the test tree with aggregated stats
type TreeNode struct {
	Name     string
	Path     string
	IsDir    bool
	Children map[string]*TreeNode
	Stats    test262.Stats
}

// findTestFiles discovers test files matching the pattern
func findTestFiles(testDir, pattern, subPath, skipPattern string, jsonMode bool) ([]string, error) {
	var testFiles []string

	err := filepath.Walk(testDir, func(path string, info os.FileInfo, err error) error {
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

		// Skip FIXTURE files - they are not tests, just data/helper files
		if matched && !strings.Contains(filepath.Base(path), "_FIXTURE") {
			// Check if this file should be skipped based on skip pattern
			if skipPattern != "" {
				// Check if the path contains the skip pattern (simple substring match)
				// or if it matches as a glob pattern
				if strings.Contains(path, skipPattern) {
					return nil // Skip this file
				}
				// Also try matching the skip pattern as a path component
				pathParts := strings.Split(path, string(filepath.Separator))
				for _, part := range pathParts {
					if part == skipPattern {
						return nil // Skip this file
					}
				}
			}
			testFiles = append(testFiles, path)
		}

		return nil
	})

	// Sort test files for consistent ordering
	sort.Strings(testFiles)

	if subPath != "" && !jsonMode {
		fmt.Printf("Searching in subdirectory: %s\n", subPath)
	}
	if skipPattern != "" && !jsonMode {
		fmt.Printf("Skipping files matching: %s\n", skipPattern)
	}

	return testFiles, err
}

// runTests executes all test files
func runTests(testFiles []string, opts runOptions, treeMode bool, suiteMode bool, disasm bool) (test262.Stats, []test262.Result) {
	var stats test262.Stats
	var fileResults []test262.Result
	stats.Total = len(testFiles)
	testDir := opts.testDir
	jsonMode := opts.jsonMode

	startTime := time.Now()

	// For tree mode, build initial tree structure and setup display
	var tree *TreeNode
	var lastDir string
	var dirFileCount = make(map[string]int)
	var dirProcessedCount = make(map[string]int)

	if treeMode || suiteMode {
		tree = &TreeNode{
			Name:     "test",
			Path:     testDir,
			IsDir:    true,
			Children: make(map[string]*TreeNode),
		}
		// Pre-build directory structure from file list and count files per directory
		for _, testFile := range testFiles {
			relPath, err := filepath.Rel(testDir, testFile)
			if err != nil {
				continue
			}
			parts := strings.Split(relPath, string(filepath.Separator))

			// Count files in the immediate parent directory
			if len(parts) > 1 {
				dirPath := strings.Join(parts[:len(parts)-1], string(filepath.Separator))
				dirFileCount[dirPath]++
			} else {
				dirFileCount["."]++
			}

			current := tree
			for _, part := range parts[:len(parts)-1] { // Skip the file itself
				if _, exists := current.Children[part]; !exists {
					current.Children[part] = &TreeNode{
						Name:     part,
						Path:     filepath.Join(current.Path, part),
						IsDir:    true,
						Children: make(map[string]*TreeNode),
					}
				}
				current = current.Children[part]
			}
		}

		// Initial display for tree mode only
		if treeMode && !jsonMode {
			fmt.Print("\033[2J\033[H") // Clear screen
			fmt.Println("\n=== Test262 Progress ===")
			fmt.Printf("Starting %d tests...\n", len(testFiles))
			fmt.Printf("\n%-60s %8s %40s\n", "Directory", "% Passed", "Total/Pass/Fail/Skip/Timeout")
			fmt.Println(strings.Repeat("-", 110))
			printColoredTreeNode(tree, "", true, false)
		}
	}

	for i, testFile := range testFiles {
		// Force GC every 100 tests to prevent excessive memory accumulation
		// Also clear the global RootShape transitions which accumulate across all tests
		if i > 0 && i%100 == 0 {
			// Clear RootShape transitions to prevent shape tree bloat
			// This is safe because each test creates fresh objects
			vm.ClearShapeCache()
			runtime.GC()
		}

		result := runFile(testFile, opts)
		for _, v := range result.Variants {
			stats.Variants++
			if v.Status == test262.StatusPass {
				stats.VariantsPassed++
			}
		}
		if !jsonMode {
			result.Error = ""
		}

		switch result.Status {
		case test262.StatusPass:
			stats.Passed++
		case test262.StatusSkip:
			stats.Skipped++
			if opts.verbose && !treeMode && !jsonMode {
				fmt.Printf("SKIP %d/%d %s\n", i+1, stats.Total, testFile)
			}
		default:
			switch result.Status {
			case test262.StatusTimeout:
				stats.Timeouts++
				// Aggressive cleanup after timeout to prevent memory bloat
				vm.ClearShapeCache()
				runtime.GC()
			case test262.StatusInfra:
				stats.InfraErrors++
			default:
				stats.Failed++
			}
			if !treeMode && !jsonMode {
				label := map[test262.Status]string{test262.StatusTimeout: "TIMEOUT", test262.StatusInfra: "INFRA", test262.StatusFail: "FAIL"}[result.Status]
				for _, v := range result.Variants {
					if v.Status != test262.StatusPass {
						fmt.Printf("%s %d/%d %s [%s] - %s\n", label, i+1, stats.Total, testFile, v.Variant, v.Diagnostic)
					}
				}
				if disasm && result.Status == test262.StatusFail {
					printDisassembly(testFile, opts)
				}
			}
			if jsonMode && result.Error == "" {
				result.Error = string(result.Status)
			}
		}

		fileResults = append(fileResults, result)

		// Update tree display in tree mode only
		if treeMode {
			relPath, _ := filepath.Rel(testDir, testFile)
			updateNodeStats(tree, relPath, result)

			// Determine current directory
			parts := strings.Split(relPath, string(filepath.Separator))
			var currentDir string
			if len(parts) > 1 {
				currentDir = strings.Join(parts[:len(parts)-1], string(filepath.Separator))
			} else {
				currentDir = "."
			}

			// Track processed files in directory
			dirProcessedCount[currentDir]++

			// Check if we've finished a directory or it's the last test
			dirComplete := dirProcessedCount[currentDir] == dirFileCount[currentDir]
			isLastTest := i == len(testFiles)-1

			// Update display when directory changes, completes, or on last test
			if ((currentDir != lastDir && lastDir != "") || dirComplete || isLastTest) && !jsonMode {
				// Clear screen and redraw tree
				fmt.Print("\033[2J\033[H") // Clear screen and move cursor to top
				fmt.Println("\n=== Test262 Progress ===")
				fmt.Printf("Progress: %d/%d tests\n", i+1, len(testFiles))
				if !isLastTest {
					fmt.Printf("Current directory: %s\n", currentDir)
				}
				fmt.Printf("\n%-60s %8s %40s\n", "Directory", "% Passed", "Total/Pass/Fail/Skip/Timeout")
				fmt.Println(strings.Repeat("-", 110))
				printColoredTreeNode(tree, "", true, false)
			}

			lastDir = currentDir
		} else if suiteMode {
			// For suite mode, still track stats but don't show live updates
			relPath, _ := filepath.Rel(testDir, testFile)
			updateNodeStats(tree, relPath, result)
		}

		// Force GC more frequently to help with memory management
		if i%100 == 99 {
			runtime.GC()
			runtime.GC() // Double GC to be more aggressive
		}
	}

	stats.Duration = time.Since(startTime)

	// Print final memory stats only if not in tree mode and not in json mode
	if !treeMode && !jsonMode {
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		memUsageMB := float64(memStats.Alloc) / 1024 / 1024
		heapMB := float64(memStats.HeapAlloc) / 1024 / 1024
		numGoroutines := runtime.NumGoroutine()
		fmt.Printf("\nFinal stats: [Mem: %.1fMB Heap: %.1fMB Goroutines: %d]\n",
			memUsageMB, heapMB, numGoroutines)
	}

	return stats, fileResults
}

// printDisassembly compiles a failing test (sloppy, with its includes) and
// prints the bytecode, for -disasm.
func printDisassembly(testFile string, opts runOptions) {
	raw, err := os.ReadFile(testFile)
	if err != nil {
		return
	}
	src := string(raw)
	meta, _ := parseMeta(src)
	var b strings.Builder
	for _, inc := range append([]string{"sta.js", "assert.js"}, meta.Includes...) {
		if incBytes, err := os.ReadFile(filepath.Join(opts.testRoot, "harness", inc)); err == nil {
			b.Write(incBytes)
			b.WriteString("\n")
		}
	}
	b.WriteString(src)
	pas := newTest262Paserati(testFile, &printSink{}, opts)
	defer pas.Cleanup()
	prog, _ := parser.NewParser(lexer.NewLexer(b.String())).ParseProgram()
	chunk, cerrs := pas.CompileProgram(prog)
	if len(cerrs) > 0 {
		fmt.Printf("[Disasm] compile errors: %d\n", len(cerrs))
		errorsPkg.DisplayErrors(cerrs, b.String())
		return
	}
	if chunk != nil {
		fmt.Println(chunk.DisassembleChunk(testFile))
	}
}

// extractFrontmatterHeader returns the content between the leading /*--- and ---*/ block, or empty string if none
func extractFrontmatterHeader(content string) string {
	start := strings.Index(content, "/*---")
	if start == -1 {
		return ""
	}
	end := strings.Index(content[start+5:], "---*/")
	if end == -1 {
		return ""
	}
	// slice within content
	return content[start+5 : start+5+end]
}

// printSummary prints the final test summary
func printSummary(stats *test262.Stats) {
	pct := func(n int) float64 {
		if stats.Total == 0 {
			return 0
		}
		return float64(n) / float64(stats.Total) * 100
	}
	fmt.Printf("\n=== Test262 Summary (policy %d) ===\n", test262.Policy)
	fmt.Printf("Total:    %d\n", stats.Total)
	fmt.Printf("Passed:   %d (%.1f%%)\n", stats.Passed, pct(stats.Passed))
	fmt.Printf("Failed:   %d (%.1f%%)\n", stats.Failed, pct(stats.Failed))
	fmt.Printf("Timeouts: %d (%.1f%%)\n", stats.Timeouts, pct(stats.Timeouts))
	fmt.Printf("Skipped:  %d (%.1f%%)\n", stats.Skipped, pct(stats.Skipped))
	fmt.Printf("Infra:    %d (%.1f%%)\n", stats.InfraErrors, pct(stats.InfraErrors))
	fmt.Printf("Variants: %d run, %d passed\n", stats.Variants, stats.VariantsPassed)
	fmt.Printf("Duration: %v\n", stats.Duration)
	fmt.Printf("======================\n")
}

// printGCStats prints garbage collection statistics
func printGCStats() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	fmt.Printf("\n=== Memory Statistics ===\n")
	fmt.Printf("Alloc (current):     %.2f MB\n", float64(memStats.Alloc)/1024/1024)
	fmt.Printf("TotalAlloc (total):  %.2f MB\n", float64(memStats.TotalAlloc)/1024/1024)
	fmt.Printf("Sys (from OS):       %.2f MB\n", float64(memStats.Sys)/1024/1024)
	fmt.Printf("HeapAlloc:           %.2f MB\n", float64(memStats.HeapAlloc)/1024/1024)
	fmt.Printf("HeapSys:             %.2f MB\n", float64(memStats.HeapSys)/1024/1024)
	fmt.Printf("HeapIdle:            %.2f MB\n", float64(memStats.HeapIdle)/1024/1024)
	fmt.Printf("HeapInuse:           %.2f MB\n", float64(memStats.HeapInuse)/1024/1024)
	fmt.Printf("HeapReleased:        %.2f MB\n", float64(memStats.HeapReleased)/1024/1024)
	fmt.Printf("HeapObjects:         %d\n", memStats.HeapObjects)
	fmt.Printf("NumGC:               %d\n", memStats.NumGC)
	fmt.Printf("NumForcedGC:         %d\n", memStats.NumForcedGC)
	fmt.Printf("GCCPUFraction:       %.4f\n", memStats.GCCPUFraction)
	fmt.Printf("PauseTotalNs:        %.2f ms\n", float64(memStats.PauseTotalNs)/1000000)
	if memStats.NumGC > 0 {
		fmt.Printf("LastGC:              %v\n", time.Unix(0, int64(memStats.LastGC)))
		avgPause := float64(memStats.PauseTotalNs) / float64(memStats.NumGC) / 1000000
		fmt.Printf("AvgPause:            %.2f ms\n", avgPause)
	}
	fmt.Printf("========================\n")
}

// buildTree constructs a tree from test results
func buildTree(results []test262.Result, testDir string) *TreeNode {
	root := &TreeNode{
		Name:     "test",
		Path:     testDir,
		IsDir:    true,
		Children: make(map[string]*TreeNode),
	}

	for _, result := range results {
		// Get relative path from test directory
		relPath, err := filepath.Rel(testDir, result.Path)
		if err != nil {
			continue
		}

		// Split path into components
		parts := strings.Split(relPath, string(filepath.Separator))

		// Navigate/create tree structure
		current := root
		for i, part := range parts {
			isLastPart := i == len(parts)-1

			if !isLastPart {
				// Directory node
				if _, exists := current.Children[part]; !exists {
					current.Children[part] = &TreeNode{
						Name:     part,
						Path:     filepath.Join(current.Path, part),
						IsDir:    true,
						Children: make(map[string]*TreeNode),
					}
				}
				current = current.Children[part]
			}
		}

		// Update stats for this node and all parents
		updateNodeStats(root, relPath, result)
	}

	return root
}

// updateNodeStats updates statistics for a node and all its parents
func updateNodeStats(root *TreeNode, relPath string, result test262.Result) {
	parts := strings.Split(relPath, string(filepath.Separator))
	current := root

	// Update all nodes in the path
	for i := 0; i <= len(parts); i++ {
		current.Stats.Total++
		if result.Passed {
			current.Stats.Passed++
		} else if result.Failed || result.Infra {
			current.Stats.Failed++
		} else if result.TimedOut {
			current.Stats.Timeouts++
		} else if result.Skipped {
			current.Stats.Skipped++
		}
		current.Stats.Duration += result.Duration

		if i < len(parts)-1 {
			if child, exists := current.Children[parts[i]]; exists {
				current = child
			} else {
				break
			}
		}
	}
}

// printTreeSummary prints the test results as a directory tree
func printTreeSummary(results []test262.Result, testDir string) {
	tree := buildTree(results, testDir)

	// Final display - clear screen first
	fmt.Print("\033[2J\033[H") // Clear screen and move cursor to top
	fmt.Println("\n=== Test262 Final Results ===")
	fmt.Printf("\n%-60s %8s %40s\n", "Directory", "% Passed", "Total/Pass/Fail/Skip/Timeout")
	fmt.Println(strings.Repeat("-", 110))

	printColoredTreeNode(tree, "", true, true)

	fmt.Println("\n" + strings.Repeat("=", 110))
	fmt.Printf("TOTAL: %d tests | Passed: %d (%.1f%%) | Failed: %d (%.1f%%) | Timeouts: %d (%.1f%%) | Skipped: %d (%.1f%%)\n",
		tree.Stats.Total,
		tree.Stats.Passed, float64(tree.Stats.Passed)/float64(tree.Stats.Total)*100,
		tree.Stats.Failed, float64(tree.Stats.Failed)/float64(tree.Stats.Total)*100,
		tree.Stats.Timeouts, float64(tree.Stats.Timeouts)/float64(tree.Stats.Total)*100,
		tree.Stats.Skipped, float64(tree.Stats.Skipped)/float64(tree.Stats.Total)*100)
	fmt.Printf("Duration: %v\n", tree.Stats.Duration)
}

// printSuiteSummary prints pass rates for each test suite with hierarchical subdivision
func printSuiteSummary(results []test262.Result, testDir string, testPath *string) {
	// Build a hierarchical map of suite stats (suite -> subsuite -> stats)
	suiteStats := make(map[string]map[string]*test262.Stats)

	// Define the main test suites
	mainSuites := []string{"annexB", "built-ins", "intl402", "language", "staging"}

	// Initialize the hierarchical structure
	for _, suite := range mainSuites {
		suiteStats[suite] = make(map[string]*test262.Stats)
	}
	suiteStats["other"] = make(map[string]*test262.Stats)

	// Categorize results by hierarchical suite structure
	for _, result := range results {
		relPath, err := filepath.Rel(testDir, result.Path)
		if err != nil {
			continue
		}

		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) == 0 {
			continue
		}

		// Get the full path from the original test262 root to determine the correct suite
		fullRelPath, err := filepath.Rel(filepath.Join(*testPath, "test"), result.Path)
		if err != nil {
			continue
		}

		fullParts := strings.Split(fullRelPath, string(filepath.Separator))
		if len(fullParts) == 0 {
			continue
		}

		mainSuite := fullParts[0]
		var subsuite string

		// Determine the subsuite based on the path structure
		// Use deeper nesting to show more detail (e.g., language/expressions/addition)
		if len(fullParts) >= 3 {
			// Show three levels: language/expressions/addition
			subsuite = filepath.Join(fullParts[1], fullParts[2])
		} else if len(fullParts) >= 2 {
			// Show two levels: language/expressions
			subsuite = fullParts[1]
		} else {
			// If there's no subsuite level, use the main suite name
			subsuite = mainSuite
		}

		// Initialize subsuite stats if needed
		if suiteStats[mainSuite] == nil {
			suiteStats[mainSuite] = make(map[string]*test262.Stats)
		}
		if suiteStats[mainSuite][subsuite] == nil {
			suiteStats[mainSuite][subsuite] = &test262.Stats{}
		}

		stats := suiteStats[mainSuite][subsuite]
		stats.Total++
		stats.Duration += result.Duration
		if result.Passed {
			stats.Passed++
		} else if result.Failed || result.Infra {
			stats.Failed++
		} else if result.TimedOut {
			stats.Timeouts++
		} else if result.Skipped {
			stats.Skipped++
		}
	}

	// Print header
	fmt.Println("\n=== Test262 Suite Results ===")
	fmt.Printf("%-25s %8s %8s %8s %8s %8s %8s %12s\n",
		"Suite", "Total", "Passed", "Failed", "Skip", "Timeout", "% Pass", "Duration")
	fmt.Println(strings.Repeat("-", 100))

	// Sort main suites for consistent output
	var sortedMainSuites []string
	for suite := range suiteStats {
		sortedMainSuites = append(sortedMainSuites, suite)
	}
	sort.Strings(sortedMainSuites)

	// Calculate overall totals and collect all subsuite stats for recommendations
	var overallStats test262.Stats
	var allSubsuiteStats []struct {
		mainSuite string
		subSuite  string
		stats     *test262.Stats
	}

	// Print each main suite and its subsuites
	for _, mainSuite := range sortedMainSuites {
		subsuiteMap := suiteStats[mainSuite]
		if len(subsuiteMap) == 0 {
			continue
		}

		// Calculate totals for this main suite
		var mainSuiteStats test262.Stats
		var sortedSubsuites []string

		for subsuite := range subsuiteMap {
			sortedSubsuites = append(sortedSubsuites, subsuite)
		}
		sort.Strings(sortedSubsuites)

		// Print subsuites
		for _, subsuite := range sortedSubsuites {
			stats := subsuiteMap[subsuite]
			if stats.Total == 0 {
				continue
			}

			// Add to main suite totals
			mainSuiteStats.Total += stats.Total
			mainSuiteStats.Passed += stats.Passed
			mainSuiteStats.Failed += stats.Failed
			mainSuiteStats.Skipped += stats.Skipped
			mainSuiteStats.Timeouts += stats.Timeouts
			mainSuiteStats.Duration += stats.Duration

			// Add to overall totals
			overallStats.Total += stats.Total
			overallStats.Passed += stats.Passed
			overallStats.Failed += stats.Failed
			overallStats.Skipped += stats.Skipped
			overallStats.Timeouts += stats.Timeouts
			overallStats.Duration += stats.Duration

			// Add to recommendations list
			allSubsuiteStats = append(allSubsuiteStats, struct {
				mainSuite string
				subSuite  string
				stats     *test262.Stats
			}{mainSuite, subsuite, stats})

			// Print subsuite
			var passPercent float64
			if stats.Total > 0 {
				passPercent = float64(stats.Passed) / float64(stats.Total) * 100
			}

			suiteName := mainSuite + "/" + subsuite
			fmt.Printf("%-25s %8d %8d %8d %8d %8d %7.1f%% %12v\n",
				suiteName,
				stats.Total,
				stats.Passed,
				stats.Failed,
				stats.Skipped,
				stats.Timeouts,
				passPercent,
				stats.Duration.Round(time.Millisecond))
		}

		// Print main suite totals if it has subsuites
		if mainSuiteStats.Total > 0 {
			var mainPassPercent float64
			if mainSuiteStats.Total > 0 {
				mainPassPercent = float64(mainSuiteStats.Passed) / float64(mainSuiteStats.Total) * 100
			}

			fmt.Printf("%-25s %8d %8d %8d %8d %8d %7.1f%% %12v\n",
				mainSuite+" (TOTAL)",
				mainSuiteStats.Total,
				mainSuiteStats.Passed,
				mainSuiteStats.Failed,
				mainSuiteStats.Skipped,
				mainSuiteStats.Timeouts,
				mainPassPercent,
				mainSuiteStats.Duration.Round(time.Millisecond))
		}
	}

	// Print overall totals
	fmt.Println(strings.Repeat("-", 100))
	if overallStats.Total > 0 {
		overallPassPercent := float64(overallStats.Passed) / float64(overallStats.Total) * 100
		fmt.Printf("%-25s %8d %8d %8d %8d %8d %7.1f%% %12v\n",
			"GRAND TOTAL",
			overallStats.Total,
			overallStats.Passed,
			overallStats.Failed,
			overallStats.Skipped,
			overallStats.Timeouts,
			overallPassPercent,
			overallStats.Duration.Round(time.Millisecond))
	}

	// Print suggestions for which subsuites to focus on
	fmt.Println("\n=== Subsuite Priority Recommendations ===")
	fmt.Println("Focus on subsuites with the lowest pass rates first:")
	var suitePriorities []struct {
		mainSuite string
		subSuite  string
		rate      float64
		total     int
	}

	for _, item := range allSubsuiteStats {
		if item.stats.Total > 0 {
			rate := float64(item.stats.Passed) / float64(item.stats.Total) * 100
			suitePriorities = append(suitePriorities, struct {
				mainSuite string
				subSuite  string
				rate      float64
				total     int
			}{item.mainSuite, item.subSuite, rate, item.stats.Total})
		}
	}

	// Sort by pass rate (ascending)
	sort.Slice(suitePriorities, func(i, j int) bool {
		return suitePriorities[i].rate < suitePriorities[j].rate
	})

	// Only show subsuites with significant test counts (>10 tests)
	for _, sp := range suitePriorities {
		if sp.total >= 10 {
			fmt.Printf("  %-15s/%-8s: %.1f%% pass rate (%d tests)\n", sp.mainSuite, sp.subSuite, sp.rate, sp.total)
		}
	}
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorGray   = "\033[90m"
)

// getNodeColor returns the color code based on pass rate
func getNodeColor(node *TreeNode) string {
	if node.Stats.Total == 0 {
		return colorGray
	}

	passRate := float64(node.Stats.Passed) / float64(node.Stats.Total)
	if passRate == 1.0 {
		return colorGreen
	} else if passRate > 0 {
		return colorYellow
	} else {
		return colorRed
	}
}

// printColoredTreeNode recursively prints a tree node with colors
func printColoredTreeNode(node *TreeNode, indent string, isLast bool, showDuration bool) {
	if node == nil {
		return
	}

	// Calculate pass percentage
	var passPercent string
	if node.Stats.Total > 0 {
		percent := float64(node.Stats.Passed) / float64(node.Stats.Total) * 100
		passPercent = fmt.Sprintf("%.1f%%", percent)
	} else {
		passPercent = "N/A"
	}

	// Format the stats
	stats := fmt.Sprintf("%d/%d/%d/%d/%d",
		node.Stats.Total,
		node.Stats.Passed,
		node.Stats.Failed,
		node.Stats.Skipped,
		node.Stats.Timeouts)

	if showDuration {
		stats += fmt.Sprintf(" [%v]", node.Stats.Duration.Round(time.Millisecond))
	}

	// Get color based on pass rate
	color := getNodeColor(node)

	// Print the node with proper formatting
	dirName := fmt.Sprintf("%s%s", indent, node.Name)
	fmt.Printf("%s%-60s%s %s%8s%s %40s\n",
		color, dirName, colorReset,
		color, passPercent, colorReset,
		stats)

	// Get sorted child names for consistent output
	var childNames []string
	for name := range node.Children {
		childNames = append(childNames, name)
	}
	sort.Strings(childNames)

	// Print children
	for _, name := range childNames {
		child := node.Children[name]
		newIndent := indent + "  "
		printColoredTreeNode(child, newIndent, false, showDuration)
	}
}

// handleDumpMode writes all test results to a file in +/- format
func handleDumpMode(results []test262.Result, testDir string, dumpFile string, corpus string) {
	f, err := os.Create(dumpFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating dump file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	fmt.Fprintln(f, dumpHeader(corpus))

	// Sort results by path for deterministic output
	sort.Slice(results, func(i, j int) bool {
		return results[i].Path < results[j].Path
	})

	// Write each result as + (passed) or - (failed/timeout/skipped)
	passCount := 0
	failCount := 0
	for _, result := range results {
		// Get relative path from testDir
		relPath, err := filepath.Rel(testDir, result.Path)
		if err != nil {
			relPath = result.Path
		}

		if result.Passed {
			fmt.Fprintf(f, "+%s\n", relPath)
			passCount++
		} else {
			fmt.Fprintf(f, "-%s\n", relPath)
			failCount++
		}
	}

	fmt.Printf("Dumped %d test results to %s (%d passed, %d failed)\n", len(results), dumpFile, passCount, failCount)
}

// handleDiffMode compares current results against a baseline file
func handleDiffMode(results []test262.Result, testDir string, diffFile string, dumpFile string, corpus string) {
	// Load baseline
	baseline, header, err := loadBaseline(diffFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading baseline file: %v\n", err)
		os.Exit(1)
	}
	if want := dumpHeader(corpus); header != want {
		if header == "" {
			header = "no header: a policy 1 baseline"
		}
		fmt.Fprintf(os.Stderr, "warning: %s was produced under a different policy or corpus (%s; this run is %q), so differences include scoring changes, not only runtime changes\n", diffFile, header, want)
	}

	// Build current results map
	current := make(map[string]bool) // path -> passed
	for _, result := range results {
		relPath, err := filepath.Rel(testDir, result.Path)
		if err != nil {
			relPath = result.Path
		}
		current[relPath] = result.Passed
	}

	// Compare and collect differences
	var newPasses []string
	var newFailures []string
	var onlyInBaseline []string
	var onlyInCurrent []string

	// Check all tests in baseline
	for path, baselinePassed := range baseline {
		currentPassed, exists := current[path]
		if !exists {
			onlyInBaseline = append(onlyInBaseline, path)
			continue
		}

		if !baselinePassed && currentPassed {
			// Was failing, now passing
			newPasses = append(newPasses, path)
		} else if baselinePassed && !currentPassed {
			// Was passing, now failing
			newFailures = append(newFailures, path)
		}
	}

	// Check for tests only in current
	for path := range current {
		if _, exists := baseline[path]; !exists {
			onlyInCurrent = append(onlyInCurrent, path)
		}
	}

	// Sort for deterministic output
	sort.Strings(newPasses)
	sort.Strings(newFailures)
	sort.Strings(onlyInBaseline)
	sort.Strings(onlyInCurrent)

	// Print differences
	for _, path := range newPasses {
		fmt.Printf("+%s\n", path)
	}
	for _, path := range newFailures {
		fmt.Printf("-%s\n", path)
	}

	// Print summary
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "=== Diff Summary ===\n")
	fmt.Fprintf(os.Stderr, "New passes:   %d\n", len(newPasses))
	fmt.Fprintf(os.Stderr, "New failures: %d\n", len(newFailures))
	fmt.Fprintf(os.Stderr, "Net change:   %+d\n", len(newPasses)-len(newFailures))
	if len(onlyInBaseline) > 0 {
		fmt.Fprintf(os.Stderr, "Only in baseline: %d\n", len(onlyInBaseline))
	}
	if len(onlyInCurrent) > 0 {
		fmt.Fprintf(os.Stderr, "Only in current:  %d\n", len(onlyInCurrent))
	}

	// Optionally dump current state
	if dumpFile != "" {
		handleDumpMode(results, testDir, dumpFile, corpus)
	}
}

// loadBaseline reads a baseline file and returns a map of path -> passed,
// plus its "# paserati-test262 ..." header line ("" for a legacy file).
func loadBaseline(filename string) (map[string]bool, string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, "", err
	}
	header := ""
	if first, _, _ := strings.Cut(string(data), "\n"); strings.HasPrefix(first, "# paserati-test262 ") {
		header = strings.TrimSpace(first)
	}

	baseline := make(map[string]bool)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if len(line) < 2 {
			continue
		}

		status := line[0]
		path := line[1:]

		switch status {
		case '+':
			baseline[path] = true
		case '-':
			baseline[path] = false
		}
	}

	return baseline, header, nil
}
