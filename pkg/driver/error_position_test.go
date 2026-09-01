package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	paseratiErrors "github.com/nooga/paserati/pkg/errors"
)

// positionOfDiagnostic recovers a diagnostic's structured position.
func positionOfDiagnostic(err error) (paseratiErrors.Position, bool) {
	return paseratiErrors.PositionOf(err)
}

// runFileForErrors runs a file the way cmd/paserati does - module mode, with
// the entry's own filename supplied - and returns the diagnostics without
// printing anything. dir becomes the module resolver's base.
func runFileForErrors(t *testing.T, dir, name string) []error {
	t.Helper()
	path := filepath.Join(dir, name)
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	p := NewPaseratiWithBaseDir(dir)
	_, errs := p.RunCode(string(src), RunOptions{ModuleName: name, Filename: path})
	out := make([]error, len(errs))
	for i, e := range errs {
		out[i] = e
	}
	return out
}

// TestModuleLoadFailureReportsFailingModulePosition covers #148: when an
// imported module fails to compile, the reported error's *structured* position
// - the one errors.DisplayErrors renders its caret and source snippet against
// - must point at the failing module's own line, column and file. It used to be
// synthesized from the importing frame's bytecode with the column hardcoded to
// 1 and no Source attached at all, so the snippet came from the entry script
// (via DisplayErrors' fallbackSource) even though the message text alongside it
// already named the module's real 4:11.
func TestModuleLoadFailureReportsFailingModulePosition(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.ts")
	entry := filepath.Join(dir, "entry.ts")
	mustWrite(t, bad, "export const ok = 1;\nexport const also = 2;\nexport function broken() {\n  return (;\n}\n")
	mustWrite(t, entry, "import { ok } from \"./bad.ts\";\nok;\n")

	errs := runFileForErrors(t, dir, "entry.ts")
	if len(errs) == 0 {
		t.Fatal("expected a diagnostic for the unparseable module, got none")
	}
	pos, ok := positionOfDiagnostic(errs[0])
	if !ok {
		t.Fatalf("diagnostic carries no position: %v", errs[0])
	}
	if pos.Source == nil {
		t.Fatalf("diagnostic has no Source, so DisplayErrors would fall back to the entry script: %v", errs[0])
	}
	if got := pos.Source.DisplayPath(); filepath.Base(got) != "bad.ts" {
		t.Errorf("snippet would come from %q, want the failing module bad.ts", got)
	}
	if pos.Line != 4 || pos.Column != 11 {
		t.Errorf("position is %d:%d, want 4:11 (the `return (;` in bad.ts)", pos.Line, pos.Column)
	}
	// The message already named the real location before this fix; make sure
	// it still does, so the two can't drift apart again.
	if !strings.Contains(errs[0].Error(), "4:11") {
		t.Errorf("message no longer names the real location: %v", errs[0])
	}
}

// TestRuntimeErrorInsideModuleReportsThatModulesSource covers the other half of
// #148: a runtime exception thrown inside an imported module reported a line
// number from that module's own line table but no Source, so DisplayErrors
// quoted that line number out of the entry script instead.
func TestRuntimeErrorInsideModuleReportsThatModulesSource(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib.ts")
	entry := filepath.Join(dir, "entry.ts")
	mustWrite(t, lib, "export function boom(): number {\n  const o: any = null;\n  return o.x;\n}\n")
	// Padding statements so a position taken from the entry script would land
	// on a different line than the one in lib.ts, making a mix-up visible.
	mustWrite(t, entry, "import { boom } from \"./lib.ts\";\nconst a = 1;\nconst b = 2;\nconst c = 3;\nboom();\n")

	errs := runFileForErrors(t, dir, "entry.ts")
	if len(errs) == 0 {
		t.Fatal("expected a runtime diagnostic, got none")
	}
	pos, ok := positionOfDiagnostic(errs[0])
	if !ok {
		t.Fatalf("diagnostic carries no position: %v", errs[0])
	}
	if pos.Source == nil {
		t.Fatalf("diagnostic has no Source, so DisplayErrors would quote the entry script: %v", errs[0])
	}
	if got := pos.Source.DisplayPath(); filepath.Base(got) != "lib.ts" {
		t.Errorf("snippet would come from %q, want the throwing module lib.ts", got)
	}
	if pos.Line != 3 {
		t.Errorf("position is line %d, want 3 (`return o.x;` in lib.ts)", pos.Line)
	}
}

// TestEvalSourceStaysAnonymous guards the other direction: a caller with no
// file (the REPL, paserati -e) must keep the "<eval>" identity rather than
// borrowing a name from ModuleName, which is often a synthetic specifier.
func TestEvalSourceStaysAnonymous(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)
	_, errs := p.RunCode("const o = null; o.x;", RunOptions{})
	if len(errs) == 0 {
		t.Fatal("expected a runtime diagnostic, got none")
	}
	pos, ok := positionOfDiagnostic(errs[0])
	if !ok {
		t.Fatalf("diagnostic carries no position: %v", errs[0])
	}
	if pos.Source != nil && pos.Source.DisplayPath() != "<eval>" {
		t.Errorf("eval'd code took on the identity %q", pos.Source.DisplayPath())
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
