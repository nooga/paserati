package tests

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"paserati/pkg/driver"
	"paserati/pkg/errors"
	"paserati/pkg/vm"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const modulesDebug = false

// ModuleExpectation represents the expected outcome of a module test.
type ModuleExpectation struct {
	ResultType string // "value", "runtime_error", "compile_error"
	Value      string // Expected value or error message substring
}

// parseModuleExpectation extracts the expectation from the main module's comments.
// Looks for lines like:
//	    // expect: value
//		// expect_runtime_error: message
//		// expect_compile_error: message
func parseModuleExpectation(scriptContent string) (*ModuleExpectation, error) {
	scanner := bufio.NewScanner(strings.NewReader(scriptContent))
	expectRegex := regexp.MustCompile(`^//\s*(expect(?:_runtime_error|_compile_error)?):\s*(.*)`)

	for scanner.Scan() {
		line := scanner.Text()
		matches := expectRegex.FindStringSubmatch(line)
		if len(matches) == 3 {
			expectType := matches[1]
			expectValue := strings.TrimSpace(matches[2])

			resultType := ""
			switch expectType {
			case "expect":
				resultType = "value"
			case "expect_runtime_error":
				resultType = "runtime_error"
			case "expect_compile_error":
				resultType = "compile_error"
			default:
				return nil, fmt.Errorf("unknown expectation type: %s", expectType)
			}

			return &ModuleExpectation{
				ResultType: resultType,
				Value:      expectValue,
			}, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading script content: %w", err)
	}

	return nil, fmt.Errorf("no expectation comment found (e.g., // expect: value)")
}

func TestModules(t *testing.T) {
	moduleDir := "modules"
	
	// Check if modules directory exists
	if _, err := os.Stat(moduleDir); os.IsNotExist(err) {
		t.Skipf("Module test directory %q does not exist, skipping module tests", moduleDir)
		return
	}
	
	entries, err := ioutil.ReadDir(moduleDir)
	if err != nil {
		t.Fatalf("Failed to read module directory %q: %v", moduleDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		testCasePath := filepath.Join(moduleDir, entry.Name())
		t.Run(entry.Name(), func(t *testing.T) {
			runModuleTest(t, testCasePath)
		})
	}
}

func runModuleTest(t *testing.T, testCasePath string) {
	// 1. Find the main module (should be named main.ts or index.ts)
	mainModulePath := ""
	
	for _, candidate := range []string{"main.ts", "index.ts"} {
		candidatePath := filepath.Join(testCasePath, candidate)
		if _, err := os.Stat(candidatePath); err == nil {
			mainModulePath = candidatePath
			break
		}
	}
	
	if mainModulePath == "" {
		t.Fatalf("No main module found (expected main.ts or index.ts) in %q", testCasePath)
	}

	// 2. Read main module and parse expectation
	mainContentBytes, err := ioutil.ReadFile(mainModulePath)
	if err != nil {
		t.Fatalf("Failed to read main module file %q: %v", mainModulePath, err)
	}
	mainContent := string(mainContentBytes)

	expectation, err := parseModuleExpectation(mainContent)
	if err != nil {
		t.Skipf("Failed to parse expectation in %q: %v", mainModulePath, err)
		return
	}

	// 3. Set up module system and compile
	finalValue, compileErrs, runtimeErrs := compileAndRunModules(testCasePath, mainModulePath)

	// 4. Check compile errors
	if len(compileErrs) > 0 {
		if expectation.ResultType == "compile_error" {
			// Check if any error message contains the expected substring
			found := false
			var allErrors strings.Builder
			for _, cerr := range compileErrs {
				allErrors.WriteString(cerr.Error() + "\n")
				if strings.Contains(cerr.Error(), expectation.Value) {
					found = true
				}
			}
			if !found {
				t.Errorf("Expected compile error containing %q, but got errors:\n%s", expectation.Value, allErrors.String())
			}
			return
		} else {
			var allErrors strings.Builder
			for _, cerr := range compileErrs {
				allErrors.WriteString(cerr.Kind() + "Error: " + cerr.Message() + "\n")
				allErrors.WriteString(fmt.Sprintf("    at %s:%d:%d\n\n", mainModulePath, cerr.Pos().Line, cerr.Pos().Column))
			}
			t.Fatalf("Unexpected compile errors:\n%s", allErrors.String())
		}
	} else if expectation.ResultType == "compile_error" {
		t.Fatalf("Expected compile error containing %q, but compilation succeeded.", expectation.Value)
	}

	// 5. Check runtime results
	switch expectation.ResultType {
	case "value":
		if len(runtimeErrs) > 0 {
			var allErrors strings.Builder
			for _, rerr := range runtimeErrs {
				allErrors.WriteString(rerr.Error() + "\n")
			}
			t.Errorf("Expected value %q, but got runtime errors:\n%s", expectation.Value, allErrors.String())
		} else {
			actualOutput := finalValue.Inspect()
			if actualOutput != expectation.Value {
				t.Errorf("Expected output %q, but got %q", expectation.Value, actualOutput)
			}
		}
	case "runtime_error":
		if len(runtimeErrs) == 0 {
			t.Errorf("Expected runtime error containing %q, but got no runtime errors. Final value: %s", expectation.Value, finalValue.ToString())
		} else {
			// Check if any runtime error message contains the expected substring
			found := false
			var allErrors strings.Builder
			for _, rerr := range runtimeErrs {
				allErrors.WriteString(rerr.Error() + "\n")
				if strings.Contains(rerr.Error(), expectation.Value) {
					found = true
				}
			}
			if !found {
				t.Errorf("Expected runtime error containing %q, but got errors:\n%s", expectation.Value, allErrors.String())
			}
		}
	default:
		t.Fatalf("Internal test error: Unexpected expectation type %q", expectation.ResultType)
	}
}

// compileAndRunModules sets up the module system and runs a module test
func compileAndRunModules(testCasePath, mainModulePath string) (finalValue vm.Value, compileErrs []errors.PaseratiError, runtimeErrs []errors.PaseratiError) {
	// Create Paserati instance with the test case directory as the base for module resolution
	// This avoids having to change the global working directory
	paserati := driver.NewPaseratiWithBaseDir(testCasePath)

	// Get just the filename from the main module path
	mainFileName := filepath.Base(mainModulePath)
	
	// Run the main module using module-aware execution with ./ prefix and get the return value
	moduleSpecifier := "./" + mainFileName
	finalValue, compileErrs, runtimeErrs = paserati.RunModuleWithValue(moduleSpecifier)
	
	return finalValue, compileErrs, runtimeErrs
}

// Helper function to create test modules
func CreateModuleTest(t *testing.T, testName string, modules map[string]string) string {
	testDir := filepath.Join("modules", testName)
	
	// Create test directory
	err := os.MkdirAll(testDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory %q: %v", testDir, err)
	}
	
	// Write module files
	for filename, content := range modules {
		modulePath := filepath.Join(testDir, filename)
		err := ioutil.WriteFile(modulePath, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to write module file %q: %v", modulePath, err)
		}
	}
	
	return testDir
}

// Helper function to clean up test modules
func CleanupModuleTest(t *testing.T, testDir string) {
	err := os.RemoveAll(testDir)
	if err != nil {
		t.Logf("Warning: Failed to cleanup test directory %q: %v", testDir, err)
	}
}