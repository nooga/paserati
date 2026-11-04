package driver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// checkSource type-checks src as main.ts in a fresh directory under opts and
// returns its diagnostics as "line code" entries.
func checkSource(t *testing.T, src string, opts CheckOptions) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.ts"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range NewPaserati().CheckFilesWithOptions([]string{"main.ts"}, opts) {
		if r.LoadError != nil {
			t.Fatalf("load error: %v", r.LoadError)
		}
		for _, d := range r.Diagnostics {
			got = append(got, fmt.Sprintf("%d %s", d.Pos().Line, d.Code()))
		}
	}
	return strings.Join(got, ", ")
}

// The strict family on by default covers useUnknownInCatchVariables,
// noImplicitThis (seeing a contextual this through a generic callee) and
// strictBindCallApply; diagnostics come out in source order.
func TestCheckFilesStrictFamilyDefaults(t *testing.T) {
	src := "try {} catch (e) { e.foo; }\n" +
		"const h = function () { return this; };\n" +
		"declare function on<K extends 'a'>(k: K, f: (this: string) => void): void;\n" +
		"on('a', function () { this.length; });\n" +
		"function f(this: void, a: number) {}\n" +
		"f.call(undefined, 'x');\n" +
		"export {};\n"
	if got, want := checkSource(t, src, DefaultCheckOptions()), "1 TS18046, 2 TS2683, 6 TS2345"; got != want {
		t.Errorf("strict defaults: diagnostics = %q, want %q", got, want)
	}
	if got := checkSource(t, src, CheckOptions{}); got != "" {
		t.Errorf("strict off: diagnostics = %q, want none", got)
	}
}

// noPropertyAccessFromIndexSignature: a property only an index signature
// provides must be read with brackets (TS4111); declared properties, Object's
// members and Paserati's permissive Error signature are exempt.
func TestCheckFilesNoPropertyAccessFromIndexSignature(t *testing.T) {
	src := "declare const r: { [k: string]: string; foo: string };\n" +
		"r.foo; r['bar']; r.toString(); r.constructor;\n" +
		"r.bar;\n" +
		"declare const u: { [k: string]: number } | undefined;\n" +
		"u?.baz;\n" +
		"declare const e: Error;\n" +
		"e.message;\n" +
		"export {};\n"
	opts := DefaultCheckOptions()
	opts.NoPropertyAccessFromIndexSignature = true
	if got, want := checkSource(t, src, opts), "3 TS4111, 5 TS4111"; got != want {
		t.Errorf("diagnostics = %q, want %q", got, want)
	}
	if got := checkSource(t, src, DefaultCheckOptions()); got != "" {
		t.Errorf("option off: diagnostics = %q, want none", got)
	}
}
