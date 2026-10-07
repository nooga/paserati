package driver

import (
	"fmt"
	"path/filepath"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/source"
	"github.com/nooga/paserati/pkg/vm"
)

// Program is a script that has been parsed, type-checked and compiled once and
// can then be run any number of times, on any compatible session, without
// repeating that work. Embedders that run the same code in many short-lived
// sessions (a fresh Paserati per call, or one pooled session) pay parse and
// compile on every call otherwise, which is most of the time of a small run.
//
// A Program is immutable and safe to share between goroutines. Each RunProgram
// executes a private instance of it, so runs never see each other's function
// objects, inline caches or properties.
type Program struct {
	chunk   *vm.Chunk // pristine: never run, only instantiated
	source  string
	options RunOptions
}

// Precompile compiles sourceCode as a Script (RunOptions.Script is implied;
// module-mode programs are not supported yet) using this session's compiler
// and type-checking settings, without running it.
//
// Compilation assigns global slots in this session's layout, so a Program can
// run on any session built from the same builtins whose own global layout does
// not conflict with it - typically a fresh NewPaserati(), or the session it was
// compiled on. RunProgram reports a conflict as an error rather than running.
func (p *Paserati) Precompile(sourceCode string, options RunOptions) (*Program, []errors.PaseratiError) {
	options.Script = true
	filename := scriptFilename(options)
	var sourceFile *source.SourceFile
	if options.Filename != "" {
		sourceFile = source.NewSourceFile(filepath.Base(options.Filename), options.Filename, sourceCode)
	} else {
		sourceFile = source.NewSourceFile(filepath.Base(filename), filename, sourceCode)
	}
	program, parseErrs := parser.NewParser(lexer.NewLexerWithSource(sourceFile)).ParseProgram()
	if len(parseErrs) > 0 {
		return nil, parseErrs
	}
	chunk, errs := p.compileAsScript(program)
	if len(errs) > 0 {
		return nil, errs
	}
	return &Program{chunk: chunk, source: sourceCode, options: options}, nil
}

// RunProgram runs a precompiled Program on this session as a Script. The result
// is the same as RunCode on the program's source with Script set. If the
// program contains something that cannot be given a private copy per run, it is
// transparently compiled from source instead, so the result is always correct.
func (p *Paserati) RunProgram(prog *Program) (vm.Value, []errors.PaseratiError) {
	if prog == nil || prog.chunk == nil {
		return vm.Undefined, []errors.PaseratiError{&errors.RuntimeError{Msg: "RunProgram: nil program"}}
	}
	inst, ok := vm.InstantiateChunk(prog.chunk)
	if !ok {
		return p.RunCode(prog.source, prog.options)
	}
	if err := p.prepareChunkGlobalLayout(inst); err != nil {
		return vm.Undefined, []errors.PaseratiError{err}
	}
	return p.runScriptChunk(inst, scriptFilename(prog.options))
}

func (prog *Program) String() string {
	return fmt.Sprintf("Program(%s, %d bytes of bytecode)", scriptFilename(prog.options), len(prog.chunk.Code))
}
