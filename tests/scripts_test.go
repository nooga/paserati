package tests

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"paserati/pkg/driver" // Use the new driver package
	"paserati/pkg/vm"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Expectation represents the expected outcome of a script.
type Expectation struct {
	ResultType string // "value", "runtime_error", "compile_error"
	Value      string // Expected value or error message substring
}

// parseExpectation extracts the expectation from the script's comments.
// Looks for lines like: // expect: value
//
//	// expect_runtime_error: message
//	// expect_compile_error: message
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
					for _, cerr := range compileErrs {
						if strings.Contains(cerr.Error(), expectation.Value) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected compile error containing %q, but got errors: %v", expectation.Value, compileErrs)
					}
					return // Expected compile error, test passes if found
				} else {
					t.Fatalf("Unexpected compile errors: %v", compileErrs)
				}
			} else if expectation.ResultType == "compile_error" {
				t.Fatalf("Expected compile error containing %q, but compilation succeeded.", expectation.Value)
			}

			// Should not happen if compileErrs is checked, but safeguard
			if chunk == nil {
				t.Fatalf("Compilation succeeded but returned a nil chunk unexpectedly.")
			}

			// 4. Run VM
			vmInstance := vm.NewVM()
			// Capture stdout/stderr for checking results or runtime errors
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			oldStderr := os.Stderr
			rErr, wErr, _ := os.Pipe()
			os.Stderr = wErr

			interpretResult := vmInstance.Interpret(chunk)

			w.Close()
			os.Stdout = oldStdout
			wErr.Close()
			os.Stderr = oldStderr

			var vmStdout bytes.Buffer
			_, _ = vmStdout.ReadFrom(r)
			actualOutput := strings.TrimSpace(vmStdout.String())

			var vmStderr bytes.Buffer
			_, _ = vmStderr.ReadFrom(rErr)
			actualRuntimeError := strings.TrimSpace(vmStderr.String())

			// 5. Check Runtime Results
			switch expectation.ResultType {
			case "value":
				if interpretResult != vm.InterpretOK {
					t.Errorf("Expected VM to return InterpretOK, but got %v. Stderr: %q", interpretResult, actualRuntimeError)
				}
				if actualOutput != expectation.Value {
					t.Errorf("Expected output %q, but got %q", expectation.Value, actualOutput)
				}
			case "runtime_error":
				if interpretResult == vm.InterpretOK {
					t.Errorf("Expected runtime error containing %q, but VM returned InterpretOK. Stdout: %q", expectation.Value, actualOutput)
				} else {
					// Check if stderr contains the expected message
					if !strings.Contains(actualRuntimeError, expectation.Value) {
						t.Errorf("Expected runtime error containing %q, but got stderr: %q", expectation.Value, actualRuntimeError)
					}
				}
			default:
				t.Fatalf("Internal test error: Unexpected expectation type %q", expectation.ResultType)
			}
		})
	}
}
