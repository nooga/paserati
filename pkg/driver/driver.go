package driver

import (
	"fmt"
	"io/ioutil"
	"os"
	"paserati/pkg/compiler"
	"paserati/pkg/errors"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
	"paserati/pkg/vm"
)

// CompileString takes Paserati source code as a string, compiles it,
// and returns the resulting VM chunk or an aggregated list of Paserati errors.
func CompileString(source string) (*vm.Chunk, []errors.PaseratiError) {
	l := lexer.NewLexer(source)
	p := parser.NewParser(l)
	program, parseErrs := p.ParseProgram()
	if len(parseErrs) > 0 {
		return nil, parseErrs
	}

	comp := compiler.NewCompiler()
	chunk, compileErrs := comp.Compile(program)
	if len(compileErrs) > 0 {
		return nil, compileErrs
	}

	return chunk, nil
}

// CompileFile reads a file, compiles its content, and returns the
// resulting VM chunk or an aggregated list of Paserati errors.
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
	return CompileString(source)
}

// RunString compiles and interprets Paserati source code from a string.
// It prints any errors encountered (syntax, compile, runtime) and the
// final result if execution is successful.
// Returns true if execution completed without any errors, false otherwise.
func RunString(source string) bool {
	l := lexer.NewLexer(source)
	p := parser.NewParser(l)
	program, parseErrs := p.ParseProgram()
	if len(parseErrs) > 0 {
		errors.DisplayErrors(source, parseErrs)
		return false
	}

	comp := compiler.NewCompiler()
	chunk, compileErrs := comp.Compile(program)
	if len(compileErrs) > 0 {
		errors.DisplayErrors(source, compileErrs)
		return false
	}
	if chunk == nil {
		fmt.Println("Internal Error: Compilation returned no errors but nil chunk.")
		return false
	}

	vmInstance := vm.NewVM()
	finalValue, runtimeErrs := vmInstance.Interpret(chunk)
	if len(runtimeErrs) > 0 {
		errors.DisplayErrors(source, runtimeErrs)
		return false
	}

	fmt.Println(finalValue)
	return true
}

// RunFile reads, compiles, and interprets a Paserati source file.
// It prints errors and results similar to RunString.
// Returns true if execution completed without any errors, false otherwise.
func RunFile(filename string) bool {
	sourceBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		readErr := &errors.CompileError{
			Position: errors.Position{Line: 0, Column: 0},
			Msg:      fmt.Sprintf("Failed to read file '%s': %s", filename, err.Error()),
		}
		fmt.Fprintf(os.Stderr, "%s Error: %s\n", readErr.Kind(), readErr.Message())
		return false
	}
	source := string(sourceBytes)
	return RunString(source)
}
