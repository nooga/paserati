package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runFileForValue runs name as the entry module in dir and returns the final
// value's string form, failing the test on any diagnostic.
func runFileForValue(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	p := NewPaseratiWithBaseDir(dir)
	val, errs := p.RunCode(string(src), RunOptions{ModuleName: name, Filename: path})
	if len(errs) > 0 {
		t.Fatalf("running %s: %v", name, errs[0])
	}
	return val.ToString()
}

// TestExportDeclarationExportsEveryBinding covers the two ways
// processExportDeclaration (in both the checker and the compiler) used to miss
// exported names, because it read the declaration statement's legacy
// first-declarator Name field instead of the bindings the declaration actually
// introduces:
//
//   - A destructuring declaration had no case at all, so `export const {a} = o`
//     declared a locally but exported nothing. Importers saw undefined, and a
//     namespace import omitted the name entirely - so it was the export
//     registration that was missing, not the value.
//   - A multi-declarator clause exported exactly one name, and it was the
//     *last* one, not the first: compileLetStatement aliases node.Name to each
//     declarator as it compiles it, and the declaration is compiled before
//     processExportDeclaration reads that field.
func TestExportDeclarationExportsEveryBinding(t *testing.T) {
	tests := []struct {
		name  string
		lib   string
		names []string // the bindings lib exports, in the order want lists them
		want  string
	}{
		{
			name: "object and array patterns beside a plain binding",
			lib: "export const {a} = {a: 1};\n" +
				"export const [b] = [2];\n" +
				"export const c = 3;\n",
			names: []string{"a", "b", "c"},
			want:  "1,2,3",
		},
		{
			// Every declarator of a multi-declarator clause, for each keyword.
			name: "multi-declarator let and var",
			lib: "export let a = 1, b = 2;\n" +
				"export var c = 3, d = 4;\n",
			names: []string{"a", "b", "c", "d"},
			want:  "1,2,3,4",
		},
		{
			// The shape paserati#159 made parseable: a pattern as a non-first
			// declarator. It desugars to two ExportNamedDeclarations, so both
			// halves have to register.
			name: "pattern mixed into a declarator list",
			lib: "export let a = 1, {b} = {b: 2};\n" +
				"export const {c} = {c: 3}, d = 4;\n",
			names: []string{"a", "b", "c", "d"},
			want:  "1,2,3,4",
		},
		{
			name: "renamed, elided, defaulted and nested pattern targets",
			lib: "export const {x: a, y: b} = {x: 1, y: 2};\n" +
				"export const [c, , d] = [3, 99, 4];\n" +
				"export const {e = 5} = {};\n" +
				"export const {f: {g}} = {f: {g: 6}};\n",
			names: []string{"a", "b", "c", "d", "e", "g"},
			want:  "1,2,3,4,5,6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			mustWrite(t, filepath.Join(dir, "lib.ts"), tt.lib)

			// Named imports.
			entry := "import { " + strings.Join(tt.names, ", ") + " } from \"./lib.ts\";\n" +
				"[" + strings.Join(tt.names, ", ") + "].join(\",\");\n"
			mustWrite(t, filepath.Join(dir, "entry.ts"), entry)
			if got := runFileForValue(t, dir, "entry.ts"); got != tt.want {
				t.Errorf("named imports = %q, want %q", got, tt.want)
			}

			// A namespace import must carry the same names - this is what showed
			// them missing rather than merely undefined.
			nsEntry := "import * as m from \"./lib.ts\";\n" +
				"[" + nsReads(tt.names) + "].join(\",\");\n"
			mustWrite(t, filepath.Join(dir, "ns.ts"), nsEntry)
			if got := runFileForValue(t, dir, "ns.ts"); got != tt.want {
				t.Errorf("namespace import = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestExportedPatternBindingKeepsItsType checks the checker half: the export has
// to be registered with the binding's real type, not skipped (which surfaces as
// `any` at the import site and silently accepts everything).
func TestExportedPatternBindingKeepsItsType(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "lib.ts"),
		"export const {num, str} = {num: 1, str: \"s\"};\n"+
			"export let a = 1, {flag} = {flag: true};\n")

	// The well-typed reads must pass...
	mustWrite(t, filepath.Join(dir, "ok.ts"),
		"import { num, str, flag } from \"./lib.ts\";\n"+
			"const n: number = num;\n"+
			"const s: string = str;\n"+
			"const b: boolean = flag;\n"+
			"[n, s, b].join(\",\");\n")
	if got := runFileForValue(t, dir, "ok.ts"); got != "1,s,true" {
		t.Errorf("typed reads = %q, want %q", got, "1,s,true")
	}

	// ...and a mistyped one must be caught, which it can't be if the export was
	// never registered or was registered as any.
	mustWrite(t, filepath.Join(dir, "bad.ts"),
		"import { str } from \"./lib.ts\";\n"+
			"const n: number = str;\n"+
			"n;\n")
	errs := runFileForErrors(t, dir, "bad.ts")
	if len(errs) == 0 {
		t.Fatal("assigning an exported string binding to a number was accepted")
	}
	if !strings.Contains(errs[0].Error(), "not assignable to type 'number'") {
		t.Errorf("unexpected diagnostic: %v", errs[0])
	}
}

// TestExportStarForwardsPatternBindings checks that `export * from` picks up
// pattern-bound names too: it expands via the source module's export list, so it
// only ever saw whatever processExportDeclaration had registered.
func TestExportStarForwardsPatternBindings(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "lib.ts"),
		"export const {a} = {a: 1};\n"+
			"export const [b] = [2];\n"+
			"export let c = 3, d = 4;\n")
	mustWrite(t, filepath.Join(dir, "reexport.ts"), "export * from \"./lib.ts\";\n")
	mustWrite(t, filepath.Join(dir, "entry.ts"),
		"import { a, b, c, d } from \"./reexport.ts\";\n"+
			"[a, b, c, d].join(\",\");\n")

	if got := runFileForValue(t, dir, "entry.ts"); got != "1,2,3,4" {
		t.Errorf("re-exported bindings = %q, want %q", got, "1,2,3,4")
	}
}

func nsReads(names []string) string {
	reads := make([]string, len(names))
	for i, n := range names {
		reads[i] = "m." + n
	}
	return strings.Join(reads, ", ")
}
