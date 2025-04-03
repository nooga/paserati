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
)

// Paserati represents a persistent interpreter session.
// It maintains state between separate code evaluations,
// allowing variables and functions defined in one evaluation
// to be used in subsequent ones.
type Paserati struct {
	vmInstance *vm.VM
	checker    *checker.Checker
	// We don't persist the compiler because each compilation
	// produces a fresh chunk that gets executed in the VM.
	// However, the VM maintains environment state like globals.
}

// NewPaserati creates a new Paserati session with a fresh VM and Checker.
func NewPaserati() *Paserati {
	return &Paserati{
		vmInstance: vm.NewVM(),
		checker:    checker.NewChecker(),
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
		return vm.Undefined(), parseErrs
	}

	// --- Type Checking is now done inside Compiler.Compile using the persistent checker ---
	// No need to call p.checker.Check(program) here directly.

	// --- Compilation Step ---
	comp := compiler.NewCompiler() // Create a fresh compiler
	comp.SetChecker(p.checker)     // Inject the persistent checker

	// Compile the program. Type checking now happens inside Compile
	// using the injected checker.
	chunk, compileAndTypeErrs := comp.Compile(program)
	if len(compileAndTypeErrs) > 0 {
		return vm.Undefined(), compileAndTypeErrs // Errors could be type or compile errors
	}
	if chunk == nil {
		// Handle internal compiler error where no chunk is returned despite no errors
		internalErr := &errors.RuntimeError{
			Position: errors.Position{Line: 0, Column: 0}, // Placeholder position
			Msg:      "Internal Error: Compilation returned nil chunk without errors.",
		}
		return vm.Undefined(), []errors.PaseratiError{internalErr}
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
	if !vm.IsUndefined(value) {
		fmt.Println(value)
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

	// --- Execution (fresh VM) ---
	vmInstance := vm.NewVM()
	finalValue, runtimeErrs := vmInstance.Interpret(chunk)
	if len(runtimeErrs) > 0 {
		errors.DisplayErrors(source, runtimeErrs)
		return false
	}

	// Display result similar to the session's DisplayResult
	if !vm.IsUndefined(finalValue) {
		fmt.Println(finalValue)
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
