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
