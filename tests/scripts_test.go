package tests

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"paserati/pkg/builtins"
	"paserati/pkg/checker"
	"paserati/pkg/compiler"
	"paserati/pkg/errors"
	"paserati/pkg/lexer"
	"paserati/pkg/modules"
	"paserati/pkg/parser"
	"paserati/pkg/vm"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const scriptsDebug = false

// compilerAdapter adapts compiler.Compiler to modules.Compiler interface
type compilerAdapter struct {
	*compiler.Compiler
}

// Compile adapts the return type from *vm.Chunk to interface{}
func (ca *compilerAdapter) Compile(node parser.Node) (interface{}, []errors.PaseratiError) {
	chunk, errs := ca.Compiler.Compile(node)
	return chunk, errs
}

// SetChecker adapts the parameter type from modules.TypeChecker to *checker.Checker
func (ca *compilerAdapter) SetChecker(tc modules.TypeChecker) {
	// Type assert to get the concrete checker
	if concreteChecker, ok := tc.(*checker.Checker); ok {
		ca.Compiler.SetChecker(concreteChecker)
	}
}

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

			// 2. Compile and initialize VM with coordinated globals
			chunk, vmInstance, compileErrs := compileAndInitializeVM(scriptPath)

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

			// Should not happen if compileErrs is checked, but safeguard
			if chunk == nil {
				t.Fatalf("Compilation succeeded but returned a nil chunk unexpectedly.")
			}

			if scriptsDebug {
				t.Logf("--- Disassembly [%s] ---\n%s-------------------------\n",
					file.Name(), chunk.DisassembleChunk(file.Name()))
			}

			// 4. Run VM (already initialized with coordinated globals)
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
	
	// Set up global variables in VM
	return vmInstance.SetBuiltinGlobals(globalVariables)
}

// compileAndInitializeVM compiles a file and creates a VM with coordinated global indices
func compileAndInitializeVM(scriptPath string) (*vm.Chunk, *vm.VM, []errors.PaseratiError) {
	// Read the file
	sourceBytes, err := ioutil.ReadFile(scriptPath)
	if err != nil {
		readErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Failed to read file '%s': %s", scriptPath, err.Error()),
		}
		return nil, nil, []errors.PaseratiError{readErr}
	}
	source := string(sourceBytes)
	
	// Parse
	l := lexer.NewLexer(source)
	p := parser.NewParser(l)
	program, parseErrs := p.ParseProgram()
	if len(parseErrs) > 0 {
		return nil, nil, parseErrs
	}
	
	// Create compiler and VM
	comp := compiler.NewCompiler()
	vmInstance := vm.NewVM()
	
	// Get all standard initializers for coordination
	initializers := builtins.GetStandardInitializers()
	sort.Slice(initializers, func(i, j int) bool {
		return initializers[i].Priority() < initializers[j].Priority()
	})
	
	// Initialize runtime context
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
			compileErr := &errors.CompileError{
				Position: errors.Position{Line: 0, Column: 0},
				Msg:      fmt.Sprintf("Failed to initialize %s runtime: %v", init.Name(), err),
			}
			return nil, nil, []errors.PaseratiError{compileErr}
		}
	}
	
	// Pre-populate compiler global indices in alphabetical order to match VM
	var globalNames []string
	for name := range globalVariables {
		globalNames = append(globalNames, name)
	}
	sort.Strings(globalNames)
	
	// Pre-assign global indices in the compiler to match VM ordering
	for _, name := range globalNames {
		comp.GetOrAssignGlobalIndex(name)
	}
	
	// Set up global variables in VM
	if err := vmInstance.SetBuiltinGlobals(globalVariables); err != nil {
		compileErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Failed to set VM globals: %v", err),
		}
		return nil, nil, []errors.PaseratiError{compileErr}
	}
	
	// Check if this is a module (has import/export statements)
	if hasModuleStatements(program) {
		// Create a simple module loader for testing
		config := modules.DefaultLoaderConfig()
		// Use the directory containing the script file as the base directory
		scriptDir := filepath.Dir(scriptPath)
		fsResolver := modules.NewFileSystemResolver(os.DirFS(scriptDir), scriptDir)
		moduleLoader := modules.NewModuleLoader(config, fsResolver)
		
		// Set up the checker factory for the module loader
		moduleLoader.SetCheckerFactory(func() modules.TypeChecker {
			newChecker := checker.NewChecker()
			return newChecker
		})
		
		// Set up the compiler factory for the module loader  
		moduleLoader.SetCompilerFactory(func() modules.Compiler {
			newCompiler := compiler.NewCompiler()
			
			// CRITICAL: Pre-populate module compiler with builtin global indices
			// This ensures module compilers start allocating from index 21+ (after builtins 0-20)
			var builtinNames []string
			for name := range globalVariables {
				builtinNames = append(builtinNames, name)
			}
			sort.Strings(builtinNames) // Same order as main compiler
			
			// Pre-assign builtin global indices to match VM ordering
			for _, name := range builtinNames {
				newCompiler.GetOrAssignGlobalIndex(name)
			}
			
			// Return a wrapper that adapts the return type to interface{}
			return &compilerAdapter{newCompiler}
		})
		
		// Enable module mode in compiler
		comp.EnableModuleMode(scriptPath, moduleLoader)
		
		// Set module loader in VM
		vmInstance.SetModuleLoader(moduleLoader)
	}
	
	// Compile
	chunk, compileAndTypeErrs := comp.Compile(program)
	if len(compileAndTypeErrs) > 0 {
		return nil, vmInstance, compileAndTypeErrs
	}
	
	return chunk, vmInstance, nil
}

// hasModuleStatements checks if a program contains import or export statements
func hasModuleStatements(program *parser.Program) bool {
	if program == nil || program.Statements == nil {
		return false
	}
	
	for _, stmt := range program.Statements {
		switch stmt.(type) {
		case *parser.ImportDeclaration:
			return true
		case *parser.ExportNamedDeclaration:
			return true
		case *parser.ExportAllDeclaration:
			return true
		case *parser.ExportDefaultDeclaration:
			return true
		}
	}
	return false
}
