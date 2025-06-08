package driver

import (
	"fmt"
	"io/ioutil"
	"os"
	"paserati/pkg/checker"
	"paserati/pkg/compiler"
	"paserati/pkg/errors"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
	"paserati/pkg/vm"

	_ "paserati/pkg/builtins"
)

// Paserati represents a persistent interpreter session.
// It maintains state between separate code evaluations,
// allowing variables and functions defined in one evaluation
// to be used in subsequent ones.
type Paserati struct {
	vmInstance *vm.VM
	checker    *checker.Checker
	compiler   *compiler.Compiler
}

// NewPaserati creates a new Paserati session with a fresh VM and Checker.
func NewPaserati() *Paserati {
	checker := checker.NewChecker()
	comp := compiler.NewCompiler()
	comp.SetChecker(checker)

	return &Paserati{
		vmInstance: vm.NewVM(),
		checker:    checker,
		compiler:   comp,
	}
}

// RunString compiles and executes the given source code in the current session.
// It uses the persistent type checker environment.
// Returns the result value and any errors that occurred.
func (p *Paserati) RunString(source string) (vm.Value, []errors.PaseratiError) {
	l := lexer.NewLexer(source)
	parser := parser.NewParser(l)
	// Parse into a Program node, which the checker expects
	program, parseErrs := parser.ParseProgram()
	if len(parseErrs) > 0 {
		// Convert parser errors (which might not implement PaseratiError directly)
		// TODO: Ensure parser errors conform to PaseratiError interface or wrap them.
		// For now, we'll assume they are compatible or handle later.
		// If ParseProgram already returns PaseratiError, this is fine.
		// If not, we need a conversion step here.
		// Let's assume parser.ParseProgram returns compatible errors for now.
		return vm.Undefined, parseErrs
	}

	// --- Type Checking is now done inside Compiler.Compile using the persistent checker ---
	// No need to call p.checker.Check(program) here directly.

	// --- Compilation Step ---
	chunk, compileAndTypeErrs := p.compiler.Compile(program)
	if len(compileAndTypeErrs) > 0 {
		return vm.Undefined, compileAndTypeErrs // Errors could be type or compile errors
	}
	if chunk == nil {
		// Handle internal compiler error where no chunk is returned despite no errors
		internalErr := &errors.RuntimeError{
			Position: errors.Position{Line: 0, Column: 0}, // Placeholder position
			Msg:      "Internal Error: Compilation returned nil chunk without errors.",
		}
		return vm.Undefined, []errors.PaseratiError{internalErr}
	}
	// --- End Compilation ---

	// --- Execution Step (using persistent VM) ---
	finalValue, runtimeErrs := p.vmInstance.Interpret(chunk)
	// Interpret errors are already PaseratiError
	return finalValue, runtimeErrs
}

// DisplayResult formats and prints the result value and any errors.
// Returns true if execution completed without any errors, false otherwise.
func (p *Paserati) DisplayResult(source string, value vm.Value, errs []errors.PaseratiError) bool {
	if len(errs) > 0 {
		errors.DisplayErrors(source, errs)
		return false
	}

	// Only print non-undefined results in REPL-like contexts
	if value != vm.Undefined {
		fmt.Println(value.Inspect())
	}
	return true
}

// CompileString takes Paserati source code as a string, compiles it,
// and returns the resulting VM chunk or an aggregated list of Paserati errors.
// This version does NOT use a persistent session.
func CompileString(source string) (*vm.Chunk, []errors.PaseratiError) {
	l := lexer.NewLexer(source)
	p := parser.NewParser(l)
	program, parseErrs := p.ParseProgram()
	if len(parseErrs) > 0 {
		return nil, parseErrs
	}

	// --- Type Check is handled internally by Compile when no checker is set ---
	// No need to call checker.Check() here.

	comp := compiler.NewCompiler() // Fresh compiler
	// Compile will create and use its own internal checker
	chunk, compileAndTypeErrs := comp.Compile(program)
	if len(compileAndTypeErrs) > 0 {
		return nil, compileAndTypeErrs
	}

	return chunk, nil
}

// CompileFile reads a file, compiles its content, and returns the
// resulting VM chunk or an aggregated list of Paserati errors.
// This version does NOT use a persistent session.
func CompileFile(filename string) (*vm.Chunk, []errors.PaseratiError) {
	sourceBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		readErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Failed to read file '%s': %s", filename, err.Error()),
		}
		return nil, []errors.PaseratiError{readErr}
	}
	source := string(sourceBytes)
	return CompileString(source) // Reuses the non-persistent CompileString
}

// RunString compiles and interprets Paserati source code from a string.
// It prints any errors encountered (syntax, compile, runtime) and the
// final result if execution is successful.
// Returns true if execution completed without any errors, false otherwise.
// This version does NOT use a persistent session.
func RunString(source string) bool {
	return RunStringWithOptions(source, RunOptions{})
}

// RunStringWithOptions is like RunString but accepts options for debugging output
func RunStringWithOptions(source string, options RunOptions) bool {
	// Use the non-persistent CompileString (which now handles type checking internally)
	chunk, compileOrTypeErrs := CompileString(source)
	if len(compileOrTypeErrs) > 0 {
		errors.DisplayErrors(source, compileOrTypeErrs)
		return false
	}
	if chunk == nil {
		// This case should ideally be covered by CompileString errors, but check defensively
		fmt.Println("Internal Error: Compilation returned no errors but nil chunk.")
		return false
	}

	// Show bytecode if requested
	if options.ShowBytecode {
		fmt.Println("\n=== Bytecode ===")
		fmt.Print(chunk.DisassembleChunk("<script>"))
		fmt.Println("================")
	}

	// --- Execution (fresh VM) ---
	vmInstance := vm.NewVM()
	finalValue, runtimeErrs := vmInstance.Interpret(chunk)
	if len(runtimeErrs) > 0 {
		errors.DisplayErrors(source, runtimeErrs)
		return false
	}

	// Show cache statistics if requested
	if options.ShowCacheStats {
		fmt.Println("\n=== Inline Cache Statistics ===")
		vmInstance.PrintCacheStats()
		fmt.Println("===============================")
	}

	// Display result similar to the session's DisplayResult
	if finalValue != vm.Undefined {
		fmt.Println(finalValue.Inspect())
	}
	return true
}

// RunFile reads, compiles, and interprets a Paserati source file.
// It prints errors and results similar to RunString.
// Returns true if execution completed without any errors, false otherwise.
// This version uses the non-persistent RunString.
func RunFile(filename string) bool {
	sourceBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		// Directly print file read errors
		readErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Failed to read file '%s': %s", filename, err.Error()),
		}
		fmt.Fprintf(os.Stderr, "%s Error: %s\n", readErr.Kind(), readErr.Message())
		return false
	}
	source := string(sourceBytes)
	// Delegate to the non-persistent RunString, which handles other errors and printing
	return RunString(source)
}

// EmitJavaScript parses TypeScript source and emits equivalent JavaScript code
// without type annotations and TypeScript-specific syntax.
func EmitJavaScript(source string) (string, []errors.PaseratiError) {
	l := lexer.NewLexer(source)
	p := parser.NewParser(l)
	program, parseErrs := p.ParseProgram()
	if len(parseErrs) > 0 {
		return "", parseErrs
	}

	// Create JavaScript emitter and emit JS code
	emitter := parser.NewJSEmitter()
	jsCode := emitter.Emit(program)

	return jsCode, nil
}

// EmitJavaScriptFile reads a TypeScript file and emits equivalent JavaScript code.
// It returns the JavaScript code as a string or an error list.
func EmitJavaScriptFile(filename string) (string, []errors.PaseratiError) {
	sourceBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		readErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Failed to read file '%s': %s", filename, err.Error()),
		}
		return "", []errors.PaseratiError{readErr}
	}
	source := string(sourceBytes)
	return EmitJavaScript(source)
}

// WriteJavaScriptFile reads a TypeScript file, converts it to JavaScript,
// and writes the output to a file with a .js extension.
// Returns true if successful, false otherwise.
func WriteJavaScriptFile(inputFilename string, outputFilename string) bool {
	if outputFilename == "" {
		// Default to replacing .ts with .js
		outputFilename = inputFilename
		if len(outputFilename) > 3 && outputFilename[len(outputFilename)-3:] == ".ts" {
			outputFilename = outputFilename[:len(outputFilename)-3] + ".js"
		} else {
			outputFilename = outputFilename + ".js"
		}
	}

	jsCode, errs := EmitJavaScriptFile(inputFilename)
	if len(errs) > 0 {
		// Print errors
		sourceBytes, _ := ioutil.ReadFile(inputFilename)
		source := string(sourceBytes)
		errors.DisplayErrors(source, errs)
		return false
	}

	// Write JavaScript code to the output file
	err := ioutil.WriteFile(outputFilename, []byte(jsCode), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing JavaScript file: %s\n", err)
		return false
	}

	fmt.Printf("JavaScript code written to %s\n", outputFilename)
	return true
}

// RunOptions configures optional debugging output
type RunOptions struct {
	ShowTokens     bool
	ShowAST        bool
	ShowBytecode   bool
	ShowCacheStats bool // Show inline cache statistics
}

// RunCode runs source code with the given Paserati session and options
func (p *Paserati) RunCode(source string, options RunOptions) (vm.Value, []errors.PaseratiError) {
	l := lexer.NewLexer(source)
	parser := parser.NewParser(l)
	program, parseErrs := parser.ParseProgram()
	if len(parseErrs) > 0 {
		return vm.Undefined, parseErrs
	}

	// --- Compilation Step ---
	chunk, compileAndTypeErrs := p.compiler.Compile(program)
	if len(compileAndTypeErrs) > 0 {
		return vm.Undefined, compileAndTypeErrs
	}
	if chunk == nil {
		internalErr := &errors.RuntimeError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      "Internal Error: Compilation returned nil chunk without errors.",
		}
		return vm.Undefined, []errors.PaseratiError{internalErr}
	}

	// Show bytecode if requested
	if options.ShowBytecode {
		fmt.Println("\n=== Bytecode ===")
		fmt.Print(chunk.DisassembleChunk("<script>"))
		fmt.Println("================")
	}

	// --- Execution Step (using persistent VM) ---
	finalValue, runtimeErrs := p.vmInstance.Interpret(chunk)

	// Show cache statistics if requested
	if options.ShowCacheStats {
		fmt.Println("\n=== Inline Cache Statistics ===")
		p.vmInstance.PrintCacheStats()
		fmt.Println("===============================")
	}

	return finalValue, runtimeErrs
}

// GetCacheStats returns extended cache statistics from the VM instance
func (p *Paserati) GetCacheStats() vm.ExtendedCacheStats {
	return vm.GetExtendedStatsFromVM(p.vmInstance)
}
