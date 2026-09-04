package driver

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAbsolutePathEntryResolvesRelativeImport covers paserati#232, filed as
// "top-level await unsupported in .mjs entry files" - the reporter's actual
// script was `const mod = await import("./helper.js")` at module top level,
// and the failure ("no resolver could handle specifier: ./helper.js") sat
// right next to the `await`, but had nothing to do with it.
//
// The real cause: cmd/paserati's runFileWithTypes passes the CLI's filename
// argument through verbatim as both RunOptions.ModuleName and .Filename,
// which becomes the "fromPath" a relative import is resolved against. When
// that argument is an absolute path (as it commonly is - many shells and
// wrapper scripts invoke tools by absolute path), the module resolver's
// os.DirFS-backed filesystem rejected it outright: fs.FS's contract forbids
// any path starting with "/", including ones arrived at by joining a
// relative specifier onto an absolute fromPath's directory. Switching to
// modules.NewOSFileSystemResolver (real OS file access, no fs.FS path
// restrictions) fixes it - this test locks that in without needing await or
// dynamic import at all, since neither was ever the actual defect.
func TestAbsolutePathEntryResolvesRelativeImport(t *testing.T) {
	dir := t.TempDir()

	helperPath := filepath.Join(dir, "helper.mjs")
	if err := os.WriteFile(helperPath, []byte(`export const value = 42;`), 0644); err != nil {
		t.Fatalf("failed to write helper file: %v", err)
	}

	entryPath := filepath.Join(dir, "entry.mjs")
	entrySource := `
		import { value } from "./helper.mjs";
		value;
	`
	if err := os.WriteFile(entryPath, []byte(entrySource), 0644); err != nil {
		t.Fatalf("failed to write entry file: %v", err)
	}

	if !filepath.IsAbs(entryPath) {
		t.Fatalf("test setup bug: expected entryPath to be absolute, got %q", entryPath)
	}

	p := NewPaserati()
	res, errs := p.RunCode(entrySource, RunOptions{ModuleName: entryPath, Filename: entryPath})
	if len(errs) > 0 {
		t.Fatalf("unexpected errors resolving a relative import from an absolute-path entry module: %v", errs)
	}
	if got := res.ToFloat(); got != 42 {
		t.Fatalf("expected the imported value 42, got %v", res.Inspect())
	}
}
