package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"paserati/pkg/builtins"
	"paserati/pkg/driver"
	errorsPkg "paserati/pkg/errors"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"time"
)

func main() {
	// Parse command line flags
	var (
		testPath   = flag.String("path", "", "Path to test262 directory")
		pattern    = flag.String("pattern", "*.js", "File pattern for test files")
		subPath    = flag.String("subpath", "", "Subdirectory pattern within test/ (e.g., 'language/**', 'built-ins/Array/**')")
		verbose    = flag.Bool("verbose", false, "Verbose output")
		limit      = flag.Int("limit", 0, "Limit number of tests to run (0 = no limit)")
		timeout    = flag.Duration("timeout", 5*time.Second, "Timeout per test (e.g., 5s, 1m)")
		memprofile = flag.String("memprofile", "", "Write memory profile to file")
		cpuprofile = flag.String("cpuprofile", "", "Write CPU profile to file")
		gcstats    = flag.Bool("gcstats", false, "Print garbage collection statistics")
		treeMode   = flag.Bool("tree", false, "Show results as directory tree with aggregated stats")
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

	fmt.Printf("Running Test262 suite from: %s\n", *testPath)

	// Find test files
	searchDir := testDir
	if *subPath != "" {
		searchDir = filepath.Join(testDir, *subPath)
		// Remove ** from the end if present for directory search
		searchDir = strings.TrimSuffix(searchDir, "/**")
	}

	testFiles, err := findTestFiles(searchDir, *pattern, *subPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding test files: %v\n", err)
		os.Exit(1)
	}

	if *limit > 0 && len(testFiles) > *limit {
		testFiles = testFiles[:*limit]
	}

	fmt.Printf("Found %d test files\n", len(testFiles))

	// Run tests
	stats, fileResults := runTests(testFiles, *verbose, *timeout, testDir, *testPath, *treeMode)

	// Print summary or tree
	if *treeMode {
		printTreeSummary(fileResults, testDir)
	} else {
		printSummary(&stats)
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

	// Exit with appropriate code
	if stats.Failed > 0 {
		os.Exit(1)
	}
}

// TestStats tracks test statistics
type TestStats struct {
	Total    int
	Passed   int
	Failed   int
	Timeouts int
	Skipped  int
	Duration time.Duration
}

// TestResult represents the result of a single test
type TestResult struct {
	Path     string
	Passed   bool
	Failed   bool
	TimedOut bool
	Skipped  bool
	Duration time.Duration
}

// TreeNode represents a directory in the test tree with aggregated stats
type TreeNode struct {
	Name     string
	Path     string
	IsDir    bool
	Children map[string]*TreeNode
	Stats    TestStats
}

// findTestFiles discovers test files matching the pattern
func findTestFiles(testDir, pattern, subPath string) ([]string, error) {
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

		if matched {
			testFiles = append(testFiles, path)
		}

		return nil
	})

	// Sort test files for consistent ordering
	sort.Strings(testFiles)

	if subPath != "" {
		fmt.Printf("Searching in subdirectory: %s\n", subPath)
	}

	return testFiles, err
}

// runTests executes all test files
func runTests(testFiles []string, verbose bool, timeout time.Duration, testDir string, testRoot string, treeMode bool) (TestStats, []TestResult) {
	var stats TestStats
	var fileResults []TestResult
	stats.Total = len(testFiles)

	startTime := time.Now()

	// For tree mode, build initial tree structure and setup display
	var tree *TreeNode
	var lastDir string
	var dirFileCount = make(map[string]int)
	var dirProcessedCount = make(map[string]int)

	if treeMode {
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
		// Initial display
		fmt.Print("\033[2J\033[H") // Clear screen
		fmt.Println("\n=== Test262 Progress ===")
		fmt.Printf("Starting %d tests...\n", len(testFiles))
		fmt.Printf("\n%-60s %8s %40s\n", "Directory", "% Passed", "Total/Pass/Fail/Skip/Timeout")
		fmt.Println(strings.Repeat("-", 110))
		printColoredTreeNode(tree, "", true, false)
	}

	for i, testFile := range testFiles {
		testStart := time.Now()
		passed, err := runSingleTest(testFile, verbose, timeout, testDir, testRoot)
		testDuration := time.Since(testStart)

		result := TestResult{
			Path:     testFile,
			Duration: testDuration,
		}

		if err != nil {
			// Check if it's a timeout
			if strings.Contains(err.Error(), "timed out") {
				stats.Timeouts++
				result.TimedOut = true
				if !treeMode {
					fmt.Printf("TIMEOUT %d/%d %s - %v\n", i+1, stats.Total, testFile, err)
				}
			} else {
				stats.Failed++
				result.Failed = true
				if !treeMode {
					fmt.Printf("FAIL %d/%d %s - %v\n", i+1, stats.Total, testFile, err)
					// Attempt to compile and dump bytecode for debugging Unknown opcode issues
					pas := createTest262Paserati()
					defer pas.Cleanup()
					prog := parserFromFile(testFile, testRoot)
					chunk, cerrs := pas.CompileProgram(prog)
					if len(cerrs) > 0 {
						fmt.Printf("[Disasm] compile errors: %d\n", len(cerrs))
						// Print errors with includes-expanded source for clarity
						if raw, rerr := os.ReadFile(testFile); rerr == nil {
							src := string(raw)
							if hdr := extractFrontmatterHeader(src); hdr != "" {
								if includeNames := extractIncludes(hdr); len(includeNames) > 0 {
									var builder strings.Builder
									for _, inc := range includeNames {
										incPath := filepath.Join(testRoot, "harness", inc)
										if incBytes, ierr := os.ReadFile(incPath); ierr == nil {
											builder.Write(incBytes)
											builder.WriteString("\n")
										}
									}
									builder.WriteString(src)
									src = builder.String()
								}
							}
							errorsPkg.DisplayErrors(cerrs, src)
						}
						// Do not disassemble or run when compile failed
						continue
					}
					if chunk != nil {
						fmt.Println(chunk.DisassembleChunk(testFile))
					}
				}
			}
		} else if passed {
			stats.Passed++
			result.Passed = true
			// Never print passes - only show failures and timeouts
		} else {
			stats.Skipped++
			result.Skipped = true
			// Don't print skips unless verbose
			if verbose && !treeMode {
				fmt.Printf("SKIP %d/%d %s\n", i+1, stats.Total, testFile)
			}
		}

		fileResults = append(fileResults, result)

		// Update tree display in tree mode
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
			if (currentDir != lastDir && lastDir != "") || dirComplete || isLastTest {
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
		}

		// Force GC more frequently to help with memory management
		if i%100 == 99 {
			runtime.GC()
			runtime.GC() // Double GC to be more aggressive
		}
	}

	stats.Duration = time.Since(startTime)

	// Print final memory stats only if not in tree mode
	if !treeMode {
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

// runSingleTest runs a single test file with timeout
func runSingleTest(testFile string, verbose bool, timeout time.Duration, testDir string, testRoot string) (bool, error) {
	// Read test file
	content, err := os.ReadFile(testFile)
	if err != nil {
		return false, fmt.Errorf("failed to read test: %w", err)
	}

	// Skip tests with imports for now (until we have full module support)
	if strings.Contains(string(content), "import ") || strings.Contains(string(content), "export ") {
		return false, nil // Skipped
	}

	// Skip async/await tests for now (parser/runtime not implemented)
	if strings.Contains(string(content), "async function") || strings.Contains(string(content), "await ") {
		return false, nil // Skipped
	}

	// Skip files under test/async* helper directories explicitly if present in path
	relPath, _ := filepath.Rel(testDir, testFile)
	if strings.Contains(relPath, string(filepath.Separator)+"asyncHelpers") {
		return false, nil // Skipped
	}

	// Create context with timeout to properly cancel goroutines
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel() // Always cancel to free resources

	// Channel to receive test result
	type testResult struct {
		passed bool
		err    error
	}
	resultChan := make(chan testResult, 1)

	// Create Test262-enabled Paserati instance outside goroutine so we can clean it up on timeout
	paserati := createTest262Paserati()

	// IMPORTANT: This goroutine can leak if paserati.RunString gets stuck in an infinite loop.
	// Since paserati.RunString doesn't support context cancellation, we cannot interrupt it.
	// This is a known limitation that needs to be fixed in the VM/parser/checker to support
	// cancellable execution.
	go func() {
		defer func() {
			// Ensure we don't leak goroutines on panic
			if r := recover(); r != nil {
				resultChan <- testResult{passed: false, err: fmt.Errorf("test panicked: %v", r)}
			}
			// Clean up in goroutine too in case of normal completion
			paserati.Cleanup()
		}()

		// Execute the test with harness includes (if any)
		sourceWithIncludes := string(content)
		if hdr := extractFrontmatterHeader(sourceWithIncludes); hdr != "" {
			if includeNames := extractIncludes(hdr); len(includeNames) > 0 {
				var builder strings.Builder
				for _, inc := range includeNames {
					incPath := filepath.Join(testRoot, "harness", inc)
					incBytes, err := os.ReadFile(incPath)
					if err != nil {
						resultChan <- testResult{passed: false, err: fmt.Errorf("failed to read include %s: %v", inc, err)}
						return
					}
					builder.WriteString("\n// [included] ")
					builder.WriteString(inc)
					builder.WriteString("\n")
					builder.Write(incBytes)
					builder.WriteString("\n")
				}
				builder.WriteString("\n// [test body]\n")
				builder.WriteString(sourceWithIncludes)
				sourceWithIncludes = builder.String()
			}
		}

		// Parse once, compile once, execute that exact chunk
		lx := lexer.NewLexer(sourceWithIncludes)
		p := parser.NewParser(lx)
		prog, parseErrs := p.ParseProgram()
		if len(parseErrs) > 0 {
			// Negative tests that expect SyntaxError are handled as failures unless marked
			if isNegativeTest(string(content)) {
				resultChan <- testResult{passed: true, err: nil}
				return
			}
			errorsPkg.DisplayErrors(parseErrs, sourceWithIncludes)
			resultChan <- testResult{passed: false, err: fmt.Errorf("test failed: %v", parseErrs[0])}
			return
		}

		chunk, compileErrs := paserati.CompileProgram(prog)
		if len(compileErrs) > 0 {
			if isNegativeTest(string(content)) {
				resultChan <- testResult{passed: true, err: nil}
				return
			}
			errorsPkg.DisplayErrors(compileErrs, sourceWithIncludes)
			resultChan <- testResult{passed: false, err: fmt.Errorf("test failed: %v", compileErrs[0])}
			return
		}

		// Execute compiled chunk
		_, runtimeErrs := paserati.InterpretChunk(chunk)
		if len(runtimeErrs) > 0 {
			if isNegativeTest(string(content)) {
				resultChan <- testResult{passed: true, err: nil}
				return
			}
			// Show disassembly of the exact chunk that ran to debug 0xFFFF issues
			fmt.Println(chunk.DisassembleChunk(testFile))
			errorsPkg.DisplayErrors(runtimeErrs, sourceWithIncludes)
			resultChan <- testResult{passed: false, err: fmt.Errorf("test failed: %v", runtimeErrs[0])}
			return
		}

		resultChan <- testResult{passed: true, err: nil}
	}()

	// Wait for result or timeout
	select {
	case result := <-resultChan:
		return result.passed, result.err
	case <-ctx.Done():
		// Context timeout - clean up Paserati instance to reduce memory leak
		// Note: The goroutine will continue running but at least we free some memory
		paserati.Cleanup()
		return false, fmt.Errorf("test timed out after %v", timeout)
	}
}

// helper: build a parser.Program from a file
func parserFromFile(path string, testDir string) *parser.Program {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return &parser.Program{}
	}
	// Honor includes for better parity
	content := string(bytes)
	if hdr := extractFrontmatterHeader(content); hdr != "" {
		if includeNames := extractIncludes(hdr); len(includeNames) > 0 {
			var b strings.Builder
			for _, inc := range includeNames {
				incPath := filepath.Join(testDir, "harness", inc)
				if incBytes, e := os.ReadFile(incPath); e == nil {
					b.WriteString("\n")
					b.Write(incBytes)
					b.WriteString("\n")
				}
			}
			b.WriteString(content)
			content = b.String()
		}
	}
	lx := lexer.NewLexer(content)
	p := parser.NewParser(lx)
	prog, _ := p.ParseProgram()
	return prog
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

// extractIncludes parses an includes: [a.js, b.js] list from the header block
func extractIncludes(header string) []string {
	// Look for "includes:" and then capture everything inside the next [...] pair
	idx := strings.Index(header, "includes:")
	if idx == -1 {
		return nil
	}
	rest := header[idx+len("includes:"):]
	// find '[' and matching ']'
	open := strings.Index(rest, "[")
	if open == -1 {
		return nil
	}
	close := strings.Index(rest[open+1:], "]")
	if close == -1 {
		return nil
	}
	inside := rest[open+1 : open+1+close]
	// Split by commas
	parts := strings.Split(inside, ",")
	var out []string
	for _, p := range parts {
		name := strings.TrimSpace(p)
		name = strings.TrimPrefix(name, "'")
		name = strings.TrimSuffix(name, "'")
		name = strings.TrimPrefix(name, "\"")
		name = strings.TrimSuffix(name, "\"")
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

// createTest262Paserati creates a Paserati instance with Test262 builtins
func createTest262Paserati() *driver.Paserati {
	// Create a custom Paserati instance with Test262 initializers
	paserati := driver.NewPaseratiWithInitializers(getTest262EnabledInitializers())
	// Disable type checking errors for test262 (JavaScript test suite)
	paserati.SetIgnoreTypeErrors(true)
	return paserati
}

// getTest262EnabledInitializers returns standard initializers plus Test262 ones
func getTest262EnabledInitializers() []builtins.BuiltinInitializer {
	// Get standard initializers
	initializers := builtins.GetStandardInitializers()

	// Add Test262 initializers
	test262Initializers := GetTest262Initializers()
	initializers = append(initializers, test262Initializers...)

	return initializers
}

// isNegativeTest checks if a test is expected to fail
func isNegativeTest(content string) bool {
	// Simple heuristic: look for negative test markers
	return strings.Contains(content, "negative:") ||
		strings.Contains(content, "* @negative") ||
		strings.Contains(content, "SyntaxError") && strings.Contains(content, "expected")
}

// printSummary prints the final test summary
func printSummary(stats *TestStats) {
	fmt.Printf("\n=== Test262 Summary ===\n")
	fmt.Printf("Total:    %d\n", stats.Total)
	fmt.Printf("Passed:   %d (%.1f%%)\n", stats.Passed, float64(stats.Passed)/float64(stats.Total)*100)
	fmt.Printf("Failed:   %d (%.1f%%)\n", stats.Failed, float64(stats.Failed)/float64(stats.Total)*100)
	fmt.Printf("Timeouts: %d (%.1f%%)\n", stats.Timeouts, float64(stats.Timeouts)/float64(stats.Total)*100)
	fmt.Printf("Skipped:  %d (%.1f%%)\n", stats.Skipped, float64(stats.Skipped)/float64(stats.Total)*100)
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
func buildTree(results []TestResult, testDir string) *TreeNode {
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
func updateNodeStats(root *TreeNode, relPath string, result TestResult) {
	parts := strings.Split(relPath, string(filepath.Separator))
	current := root

	// Update all nodes in the path
	for i := 0; i <= len(parts); i++ {
		current.Stats.Total++
		if result.Passed {
			current.Stats.Passed++
		} else if result.Failed {
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
func printTreeSummary(results []TestResult, testDir string) {
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
