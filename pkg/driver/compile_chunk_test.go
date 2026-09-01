package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/source"
)

func TestCompileStringChunkRunsOnFreshStandardSession(t *testing.T) {
	chunk, compileErrs := CompileString(`const a = new Array(4); a.length;`)
	if len(compileErrs) != 0 {
		t.Fatalf("CompileString failed: %v", compileErrs)
	}

	result, runtimeErrs := NewPaserati().InterpretChunk(chunk)
	if len(runtimeErrs) != 0 {
		t.Fatalf("InterpretChunk failed: %v", runtimeErrs)
	}
	if got := result.ToString(); got != "4" {
		t.Fatalf("expected 4, got %s", got)
	}
}

func TestCompileFileChunkRunsOnFreshStandardSession(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "array.ts")
	if err := os.WriteFile(filename, []byte(`const a = new Array(4); a.length;`), 0o600); err != nil {
		t.Fatal(err)
	}
	chunk, compileErrs := CompileFile(filename)
	if len(compileErrs) != 0 {
		t.Fatalf("CompileFile failed: %v", compileErrs)
	}

	result, runtimeErrs := NewPaserati().InterpretChunk(chunk)
	if len(runtimeErrs) != 0 {
		t.Fatalf("InterpretChunk failed: %v", runtimeErrs)
	}
	if got := result.ToString(); got != "4" {
		t.Fatalf("expected 4, got %s", got)
	}
}

func TestInterpretChunkRejectsCustomGlobalCollision(t *testing.T) {
	chunk, compileErrs := CompileString(`let userGlobal = 1; userGlobal;`)
	if len(compileErrs) != 0 {
		t.Fatalf("CompileString failed: %v", compileErrs)
	}

	inits := append(builtins.GetStandardInitializers(), NewProcessInitializer(nil))
	p := NewPaseratiWithInitializers(inits)
	assertProcessGlobal(t, p)

	_, runtimeErrs := p.InterpretChunk(chunk)
	if len(runtimeErrs) != 1 || !strings.Contains(runtimeErrs[0].Error(), "incompatible global layout") {
		t.Fatalf("expected incompatible-layout error, got %v", runtimeErrs)
	}
	assertProcessGlobal(t, p)
}

func TestInterpretChunkAllowsExtraCustomGlobalsWithoutCollision(t *testing.T) {
	chunk, compileErrs := CompileString(`new Array(2).length;`)
	if len(compileErrs) != 0 {
		t.Fatalf("CompileString failed: %v", compileErrs)
	}

	inits := append(builtins.GetStandardInitializers(), NewProcessInitializer(nil))
	p := NewPaseratiWithInitializers(inits)
	result, runtimeErrs := p.InterpretChunk(chunk)
	if len(runtimeErrs) != 0 {
		t.Fatalf("InterpretChunk failed: %v", runtimeErrs)
	}
	if got := result.ToString(); got != "2" {
		t.Fatalf("expected 2, got %s", got)
	}
	assertProcessGlobal(t, p)
}

func TestInterpretChunkAllowsMatchingCustomLayouts(t *testing.T) {
	inits := append(builtins.GetStandardInitializers(), NewProcessInitializer(nil))
	compilerSession := NewPaseratiWithInitializers(inits)
	program := parseChunkTestProgram(t, `let userGlobal = 1; userGlobal;`)
	chunk, compileErrs := compilerSession.CompileProgram(program)
	if len(compileErrs) != 0 {
		t.Fatalf("CompileProgram failed: %v", compileErrs)
	}

	runtimeSession := NewPaseratiWithInitializers(inits)
	result, runtimeErrs := runtimeSession.InterpretChunk(chunk)
	if len(runtimeErrs) != 0 {
		t.Fatalf("InterpretChunk failed: %v", runtimeErrs)
	}
	if got := result.ToString(); got != "1" {
		t.Fatalf("expected 1, got %s", got)
	}
	assertProcessGlobal(t, runtimeSession)
}

func assertProcessGlobal(t *testing.T, p *Paserati) {
	t.Helper()
	process, exists := p.GetVM().GetGlobal("process")
	if !exists || !process.IsObject() {
		t.Fatalf("process global was corrupted: value=%v exists=%v", process, exists)
	}
}

func parseChunkTestProgram(t *testing.T, code string) *parser.Program {
	t.Helper()
	l := lexer.NewLexerWithSource(source.NewEvalSource(code))
	p := parser.NewParser(l)
	program, parseErrs := p.ParseProgram()
	if len(parseErrs) != 0 {
		t.Fatalf("parse failed: %v", parseErrs)
	}
	return program
}
