package tests

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"paserati/pkg/driver" // Use the new driver package
	"paserati/pkg/vm"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	// Add
)

const scriptsDebug = false

// Expectation represents the expected outcome of a script.
type Expectation struct {
	ResultType string // "value", "runtime_error", "compile_error"
	Value      string // Expected value or error message substring
}

// parseExpectation extracts the expectation from the script's comments.
// Looks for lines like:
//
//	    // expect: value
//		// expect_runtime_error: message
//		// expect_compile_error: message
func parseExpectation(scriptContent string) (*Expectation, error) {
	scanner := bufio.NewScanner(strings.NewReader(scriptContent))
	expectRegex := regexp.MustCompile(`^//\s*(expect(?:_runtime_error|_compile_error)?):\s*(.*)`) // More robust regex

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
				// Should not happen with the regex, but good practice
				return nil, fmt.Errorf("unknown expectation type: %s", expectType)
			}

			return &Expectation{
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

func TestScripts(t *testing.T) {
	scriptDir := "scripts"
	files, err := ioutil.ReadDir(scriptDir)
	if err != nil {
		t.Fatalf("Failed to read script directory %q: %v", scriptDir, err)
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".ts") || file.IsDir() {
			continue
		}

		scriptPath := filepath.Join(scriptDir, file.Name())
		t.Run(file.Name(), func(t *testing.T) {
			// 1. Read Script and Parse Expectation
			scriptContentBytes, err := ioutil.ReadFile(scriptPath)
			if err != nil {
				t.Fatalf("Failed to read script file %q: %v", scriptPath, err)
			}
			scriptContent := string(scriptContentBytes)

			expectation, err := parseExpectation(scriptContent)
			if err != nil {
				t.Fatalf("Failed to parse expectation in %q: %v", scriptPath, err)
			}

			// 2. Compile using the driver
			chunk, compileErrs := driver.CompileFile(scriptPath)

			// 3. Check Compile Errors
			if len(compileErrs) > 0 {
				if expectation.ResultType == "compile_error" {
					// Check if any error message contains the expected substring
					found := false
					var allErrors strings.Builder
					for _, cerr := range compileErrs {
						allErrors.WriteString(cerr.Error() + "\n")
						if strings.Contains(cerr.Error(), expectation.Value) {
							found = true
							// Don't break, maybe log all errors?
						}
					}
					if !found {
						t.Errorf("Expected compile error containing %q, but got errors:\n%s", expectation.Value, allErrors.String())
					}
					return // Expected compile error, test passes if found (or specific message found)
				} else {
					var allErrors strings.Builder
					for _, cerr := range compileErrs {
						allErrors.WriteString(cerr.Error() + "\n")
					}
					t.Fatalf("Unexpected compile errors:\n%s", allErrors.String())
				}
			} else if expectation.ResultType == "compile_error" {
				t.Fatalf("Expected compile error containing %q, but compilation succeeded.", expectation.Value)
			}

			// Should not happen if compileErrs is checked, but safeguard
			if chunk == nil {
				t.Fatalf("Compilation succeeded but returned a nil chunk unexpectedly.")
			}

			if scriptsDebug {
				t.Logf("--- Disassembly [%s] ---\n%s-------------------------\n",
					file.Name(), chunk.DisassembleChunk(file.Name()))
			}

			// 4. Run VM
			vmInstance := vm.NewVM()
			finalValue, runtimeErrs := vmInstance.Interpret(chunk)

			// 5. Check Runtime Results
			switch expectation.ResultType {
			case "value":
				if len(runtimeErrs) > 0 {
					var allErrors strings.Builder
					for _, rerr := range runtimeErrs {
						allErrors.WriteString(rerr.Error() + "\n")
					}
					t.Errorf("Expected value %q, but got runtime errors:\n%s", expectation.Value, allErrors.String())
				} else {
					actualOutput := finalValue.String()
					if actualOutput != expectation.Value {
						t.Errorf("Expected output %q, but got %q", expectation.Value, actualOutput)
					}
				}
			case "runtime_error":
				if len(runtimeErrs) == 0 {
					t.Errorf("Expected runtime error containing %q, but got no runtime errors. Final value: %s", expectation.Value, finalValue.String())
				} else {
					// Check if any runtime error message contains the expected substring
					found := false
					var allErrors strings.Builder
					for _, rerr := range runtimeErrs {
						allErrors.WriteString(rerr.Error() + "\n")
						if strings.Contains(rerr.Error(), expectation.Value) {
							found = true
							// Don't break, log all errors?
						}
					}
					if !found {
						t.Errorf("Expected runtime error containing %q, but got errors:\n%s", expectation.Value, allErrors.String())
					}
				}
			default:
				t.Fatalf("Internal test error: Unexpected expectation type %q", expectation.ResultType)
			}
		})
	}
}
