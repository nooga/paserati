package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"paserati/pkg/builtins"
	"paserati/pkg/driver"
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
	)
	
	flag.Parse()
	
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
	stats := runTests(testFiles, *verbose, *timeout)
	
	// Print summary
	printSummary(&stats)
	
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
func runTests(testFiles []string, verbose bool, timeout time.Duration) TestStats {
	var stats TestStats
	stats.Total = len(testFiles)
	
	startTime := time.Now()
	
	for i, testFile := range testFiles {
		passed, err := runSingleTest(testFile, verbose, timeout)
		
		if err != nil {
			// Check if it's a timeout
			if strings.Contains(err.Error(), "timed out") {
				stats.Timeouts++
				fmt.Printf("TIMEOUT %d/%d %s - %v\n", i+1, stats.Total, testFile, err)
			} else {
				stats.Failed++
				fmt.Printf("FAIL %d/%d %s - %v\n", i+1, stats.Total, testFile, err)
			}
		} else if passed {
			stats.Passed++
			// Never print passes - only show failures and timeouts
		} else {
			stats.Skipped++
			// Don't print skips unless verbose
			if verbose {
				fmt.Printf("SKIP %d/%d %s\n", i+1, stats.Total, testFile)
			}
		}
		
		// Force GC more frequently to help with memory management
		if i%100 == 99 {
			runtime.GC()
			runtime.GC() // Double GC to be more aggressive
		}
	}
	
	stats.Duration = time.Since(startTime)
	
	// Print final memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memUsageMB := float64(memStats.Alloc) / 1024 / 1024
	heapMB := float64(memStats.HeapAlloc) / 1024 / 1024
	numGoroutines := runtime.NumGoroutine()
	fmt.Printf("\nFinal stats: [Mem: %.1fMB Heap: %.1fMB Goroutines: %d]\n", 
		memUsageMB, heapMB, numGoroutines)
	
	return stats
}

// runSingleTest runs a single test file with timeout
func runSingleTest(testFile string, verbose bool, timeout time.Duration) (bool, error) {
	// Read test file
	content, err := os.ReadFile(testFile)
	if err != nil {
		return false, fmt.Errorf("failed to read test: %w", err)
	}
	
	// Skip tests with imports for now (until we have full module support)
	if strings.Contains(string(content), "import ") || strings.Contains(string(content), "export ") {
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
		
		// Execute the test
		_, errs := paserati.RunString(string(content))
		
		if len(errs) > 0 {
			// Check if this was expected (negative test)
			if isNegativeTest(string(content)) {
				resultChan <- testResult{passed: true, err: nil} // Expected failure
			} else {
				resultChan <- testResult{passed: false, err: fmt.Errorf("test failed: %v", errs[0])}
			}
		} else {
			resultChan <- testResult{passed: true, err: nil}
		}
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