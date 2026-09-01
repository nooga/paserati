package driver

import (
	"os"
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

func writeIssueTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

// TestRunModuleWithValueCollectsExportsWhenReentrant covers paserati#165:
// RunModuleWithValue (and RunModule) used to gate export-value collection
// on p.compiler.IsModuleMode() - the shared, stateful compiler instance's
// *current* mode flag, which an unrelated compile in between (e.g. running
// some other script on the same Paserati instance) can silently flip away
// from what this specific module needs. The module still executed
// correctly; only ExportValues came back empty, with no error anywhere.
func TestRunModuleWithValueCollectsExportsWhenReentrant(t *testing.T) {
	dir := t.TempDir()
	writeIssueTestFile(t, dir+"/mod.mjs", `
		export function greet() { return "hi"; }
		export default { greet };
	`)

	p := NewPaseratiWithBaseDir(dir)

	// Simulate reentrancy: compile+run something unrelated on the same
	// shared p.compiler in between loading and running our target module,
	// the way a require() reached partway through executing an entry
	// script would.
	if _, errs := p.RunString(`1 + 1`); len(errs) > 0 {
		t.Fatalf("unexpected errors running unrelated script: %v", errs)
	}

	final, loadErrs, runErrs := p.RunModuleWithValue("./mod.mjs")
	if len(loadErrs) > 0 || len(runErrs) > 0 {
		t.Fatalf("unexpected errors: load=%v run=%v", loadErrs, runErrs)
	}
	if final.Type() == vm.TypeUndefined {
		t.Fatalf("expected the module's own final expression value, got undefined")
	}

	rec, err := p.LoadModule("./mod.mjs", ".")
	if err != nil {
		t.Fatalf("LoadModule error: %v", err)
	}
	exportValues := rec.GetExportValues()
	if len(exportValues) == 0 {
		t.Fatalf("expected non-empty export values, got %v", exportValues)
	}
	if greet, ok := exportValues["greet"]; !ok || !greet.IsCallable() {
		t.Fatalf("expected a callable 'greet' export, got %v (ok=%v)", exportValues["greet"], ok)
	}
	if _, ok := exportValues["default"]; !ok {
		t.Fatalf("expected a 'default' export, got %v", exportValues)
	}
}

// TestRunModuleWithValueBarrelReexportOfImport covers the specific gap
// left after the first pass at paserati#165: a module whose export is
// itself a re-export of an imported binding (the barrel/index.js pattern -
// import { X } from "./base"; export { X };, see paserati#163) never
// occupies a local global slot, so ModuleRecord.ExportIndices alone can't
// see it. It only resolves through the VM's own module-context machinery
// (VM.GetModuleExport).
func TestRunModuleWithValueBarrelReexportOfImport(t *testing.T) {
	dir := t.TempDir()
	writeIssueTestFile(t, dir+"/base.mjs", `export default class Diff { kind() { return "diff"; } }`)
	writeIssueTestFile(t, dir+"/index.mjs", `
		import Diff from "./base.mjs";
		export { Diff };
	`)

	p := NewPaseratiWithBaseDir(dir)

	rec, err := p.LoadModule("./index.mjs", ".")
	if err != nil {
		t.Fatalf("LoadModule error: %v", err)
	}

	if _, loadErrs, runErrs := p.RunModuleWithValue("./index.mjs"); len(loadErrs) > 0 || len(runErrs) > 0 {
		t.Fatalf("unexpected errors: load=%v run=%v", loadErrs, runErrs)
	}

	exportValues := rec.GetExportValues()
	diffCtor, ok := exportValues["Diff"]
	if !ok {
		t.Fatalf("expected a 'Diff' export, got %v", exportValues)
	}
	if !diffCtor.IsCallable() {
		t.Fatalf("expected 'Diff' export to be a callable constructor, got %v", diffCtor.Inspect())
	}
}

// TestRunModuleWithValueNativeModuleExportsSurvive guards against a
// regression introduced while fixing paserati#165: RunModule/RunModuleWithValue
// still compile+run a native module's placeholder empty AST (it has no
// ExportIndices/ReExports of its own to derive anything from), so blindly
// recomputing ExportValues from those empty maps would silently wipe out
// the real exports handleNativeModuleSource already populated at load time.
func TestRunModuleWithValueNativeModuleExportsSurvive(t *testing.T) {
	p := NewPaserati()
	p.DeclareModule("probe165native", func(m *ModuleBuilder) {
		m.Const("ANSWER", 42.0)
	})

	rec, err := p.LoadModule("probe165native", ".")
	if err != nil {
		t.Fatalf("LoadModule error: %v", err)
	}
	if len(rec.GetExportValues()) == 0 {
		t.Fatalf("expected native module to already have exports right after loading")
	}

	if _, loadErrs, runErrs := p.RunModuleWithValue("probe165native"); len(loadErrs) > 0 || len(runErrs) > 0 {
		t.Fatalf("unexpected errors: load=%v run=%v", loadErrs, runErrs)
	}

	exportValues := rec.GetExportValues()
	if _, ok := exportValues["ANSWER"]; !ok {
		t.Fatalf("expected native module's real exports to survive RunModuleWithValue, got %v", exportValues)
	}
}
