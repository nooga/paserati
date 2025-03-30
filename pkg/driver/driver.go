package driver

import (
	"fmt"
	"io/ioutil"
	"paserati/pkg/compiler"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
	"paserati/pkg/vm"
)

// CompileString takes Paserati source code as a string, compiles it,
// and returns the resulting VM chunk or an aggregated list of errors.
func CompileString(source string) (*vm.Chunk, []error) {
	l := lexer.NewLexer(source)
	p := parser.NewParser(l)
	program := p.ParseProgram()
	parserErrors := p.Errors()
	if len(parserErrors) > 0 {
		errors := make([]error, len(parserErrors))
		for i, msg := range parserErrors {
			errors[i] = fmt.Errorf("parse error: %s", msg)
		}
		return nil, errors
	}

	comp := compiler.NewCompiler()
	chunk, compilerErrorStrings := comp.Compile(program)
	if len(compilerErrorStrings) > 0 {
		errors := make([]error, len(compilerErrorStrings))
		for i, msg := range compilerErrorStrings {
			errors[i] = fmt.Errorf("compile error: %s", msg)
		}
		// Even with errors, compiler might return a partial chunk,
		// but we prefer returning nil if there were errors.
		return nil, errors
	}

	return chunk, nil // No errors
}

// CompileFile reads a file, compiles its content, and returns the
// resulting VM chunk or an aggregated list of errors.
func CompileFile(filename string) (*vm.Chunk, []error) {
	sourceBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, []error{fmt.Errorf("file read error: %w", err)}
	}
	source := string(sourceBytes)
	return CompileString(source)
}

// TODO: Add RunString/RunFile functions later which compile and then interpret.
