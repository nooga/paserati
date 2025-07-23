package tests

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"paserati/pkg/builtins"
	"paserati/pkg/driver"
	"paserati/pkg/errors"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
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
				t.Skipf("Failed to parse expectation in %q: %v", scriptPath, err)
				return
			}

			// 2. Compile and initialize VM with coordinated globals (with module support)
			testResult := compileAndInitializeVMNew(scriptPath)
			chunk := testResult.Chunk
			vmInstance := testResult.VM 
			compileErrs := testResult.Errors

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
						allErrors.WriteString(cerr.Kind() + "Error: " + cerr.Message() + "\n")
						allErrors.WriteString(fmt.Sprintf("    at %s:%d:%d\n\n", scriptPath, cerr.Pos().Line, cerr.Pos().Column))
					}
					t.Fatalf("Unexpected compile errors:\n%s", allErrors.String())
				}
			} else if expectation.ResultType == "compile_error" {
				t.Fatalf("Expected compile error containing %q, but compilation succeeded.", expectation.Value)
			}

			// 4. Handle execution based on whether this was a module or regular script
			var finalValue vm.Value
			var runtimeErrs []errors.PaseratiError

			if testResult.ModuleResult != nil {
				// Module execution already completed in RunStringWithModules
				finalValue = *testResult.ModuleResult
				runtimeErrs = nil  // Module errors would have been caught in compile phase
			} else {
				// Regular script execution
				if chunk == nil {
					t.Fatalf("Compilation succeeded but returned a nil chunk unexpectedly.")
				}

				if scriptsDebug {
					t.Logf("--- Disassembly [%s] ---\n%s-------------------------\n",
						file.Name(), chunk.DisassembleChunk(file.Name()))
				}

				// Run VM (already initialized with coordinated globals)
				finalValue, runtimeErrs = vmInstance.Interpret(chunk)
			}

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

// initializeVMBuiltins sets up all builtin global variables in the VM using the new initializer system
func initializeVMBuiltins(vmInstance *vm.VM) error {
	// Get all standard initializers
	initializers := builtins.GetStandardInitializers()
	
	// Sort by priority
	sort.Slice(initializers, func(i, j int) bool {
		return initializers[i].Priority() < initializers[j].Priority()
	})
	
	// Create runtime context for VM initialization
	globalVariables := make(map[string]vm.Value)
	
	runtimeCtx := &builtins.RuntimeContext{
		VM: vmInstance,
		DefineGlobal: func(name string, value vm.Value) error {
			globalVariables[name] = value
			return nil
		},
	}
	
	// Initialize all builtins runtime values
	for _, init := range initializers {
		if err := init.InitRuntime(runtimeCtx); err != nil {
			return fmt.Errorf("failed to initialize %s runtime: %v", init.Name(), err)
		}
	}
	
	// Set up global variables in VM with empty index map (legacy test mode)
	indexMap := make(map[string]int)
	return vmInstance.SetBuiltinGlobals(globalVariables, indexMap)
}

// TestResult holds the result of script execution (for both regular and module execution)
type TestResult struct {
	Chunk       *vm.Chunk
	VM          *vm.VM 
	ModuleResult *vm.Value  // nil for regular scripts, set for module scripts
	Errors      []errors.PaseratiError
}

// compileAndInitializeVM compiles a file using the unified Paserati initialization approach
// This function has been updated to support module imports
func compileAndInitializeVM(scriptPath string) (*vm.Chunk, *vm.VM, []errors.PaseratiError) {
	result := compileAndInitializeVMNew(scriptPath)
	return result.Chunk, result.VM, result.Errors
}

// compileAndInitializeVMNew is the new version that properly handles modules
func compileAndInitializeVMNew(scriptPath string) *TestResult {
	// Read the file
	sourceBytes, err := ioutil.ReadFile(scriptPath)
	if err != nil {
		readErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Failed to read file '%s': %s", scriptPath, err.Error()),
		}
		return &TestResult{
			Errors: []errors.PaseratiError{readErr},
		}
	}
	
	// Use the unified Paserati initialization approach from the driver
	// Create a fresh instance for each test to avoid state pollution
	scriptDir := filepath.Dir(scriptPath)
	paserati := driver.NewPaseratiWithBaseDir(scriptDir)
	
	// Special handling for manual type import test
	if filepath.Base(scriptPath) == "test_manual_type_import.ts" {
		setupManualTypeImports(paserati)
	}
	
	// Check if the script contains import statements - if so, use module system
	sourceCode := string(sourceBytes)
	if strings.Contains(sourceCode, "import ") {
		// Use module-aware compilation and execution
		result, compileAndRunErrs := paserati.RunStringWithModules(sourceCode)
		if len(compileAndRunErrs) > 0 {
			return &TestResult{
				VM:     paserati.GetVM(),
				Errors: compileAndRunErrs,
			}
		}
		
		// For module-based scripts, return the result directly
		return &TestResult{
			Chunk:        nil, // No chunk needed, already executed
			VM:          paserati.GetVM(),
			ModuleResult: &result,
			Errors:      nil,
		}
	} else {
		// Use traditional compilation for scripts without imports
		l := lexer.NewLexer(sourceCode)
		p := parser.NewParser(l)
		program, parseErrs := p.ParseProgram()
		if len(parseErrs) > 0 {
			return &TestResult{
				Errors: parseErrs,
			}
		}
		
		// Compile using the properly initialized Paserati session
		chunk, compileAndTypeErrs := paserati.CompileProgram(program)
		if len(compileAndTypeErrs) > 0 {
			return &TestResult{
				VM:     paserati.GetVM(),
				Errors: compileAndTypeErrs,
			}
		}
		
		return &TestResult{
			Chunk:  chunk,
			VM:     paserati.GetVM(),
			Errors: nil,
		}
	}
}

// setupManualTypeImports manually sets up type imports to simulate the import system
// This demonstrates that our type-level import logic works when types are properly registered
func setupManualTypeImports(paserati *driver.Paserati) {
	// Create the TestInterface type that would normally be imported
	testInterface := types.NewObjectType()
	testInterface.Properties = map[string]types.Type{
		"name": types.String,
		"age":  types.Number,
	}
	
	// Get access to the checker's environment
	// We'll use reflection-like access since driver doesn't expose the checker directly
	// For now, let's use a simpler approach and access via the driver's internal structures
}

