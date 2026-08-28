package driver

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resolveImportMetaPathname simulates new URL(relative, import.meta.url).pathname
// using net/url, since Paserati has no WHATWG URL builtin.
func resolveImportMetaPathname(baseURL, relative string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(relative)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).Path, nil
}

func TestHostImportMetaURLFileModule(t *testing.T) {
	dir := t.TempDir()
	entryPath := filepath.Join(dir, "entry.ts")
	if err := os.WriteFile(entryPath, []byte("import.meta.url"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewPaseratiWithBaseDir(dir)
	p.SetSkipTypeCheck(true)

	val, compileErrs, runtimeErrs := p.RunModuleWithValue("./entry.ts")
	if len(compileErrs) > 0 {
		t.Fatalf("compile errors: %v", compileErrs[0])
	}
	if len(runtimeErrs) > 0 {
		t.Fatalf("runtime errors: %v", runtimeErrs[0])
	}

	got := val.ToString()
	if !strings.HasPrefix(got, "file://") {
		t.Fatalf("import.meta.url = %q, want file:// prefix", got)
	}

	absEntry, err := filepath.Abs(filepath.Join(dir, "entry.ts"))
	if err != nil {
		t.Fatal(err)
	}
	absEntry = filepath.ToSlash(absEntry)
	if !strings.Contains(got, absEntry) {
		t.Errorf("import.meta.url = %q, want to contain absolute path %q", got, absEntry)
	}

	wantFoo := filepath.ToSlash(filepath.Join(filepath.Dir(absEntry), "foo.ts"))
	pathname, err := resolveImportMetaPathname(got, "./foo.ts")
	if err != nil {
		t.Fatalf("resolveImportMetaPathname: %v", err)
	}
	if pathname != wantFoo {
		t.Errorf("new URL('./foo.ts', import.meta.url).pathname = %q, want %q", pathname, wantFoo)
	}
}

func TestHostImportMetaURLNativeModule(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)
	p.DeclareModule("fs", func(m *ModuleBuilder) {
		m.Default(nil)
	})

	val, errs := p.RunCode("import.meta.url", RunOptions{ModuleName: "native://fs"})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}
	if got := val.ToString(); got != "native://fs" {
		t.Errorf("import.meta.url = %q, want exactly native://fs", got)
	}
}

func TestHostImportMetaURLDefaultModuleNotAbsPath(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	val, errs := p.RunCode("import.meta.url", RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("RunCode failed: %v", errs[0])
	}

	got := val.ToString()
	fakeFileURL := "file://" + filepath.ToSlash(filepath.Join(cwd, "__code_module__"))
	if got == fakeFileURL || strings.HasPrefix(got, fakeFileURL) {
		t.Errorf("import.meta.url = %q, must not abs-path eval module name into cwd", got)
	}
	if strings.Contains(got, filepath.ToSlash(cwd)) && strings.Contains(got, "__code_module__") && strings.HasPrefix(got, "file://") {
		t.Errorf("import.meta.url = %q, must not turn __code_module__ into a file URL under cwd", got)
	}
}

func TestHostImportMetaURLPercentEncoding(t *testing.T) {
	dir := t.TempDir()
	modDir := filepath.Join(dir, "my mod")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entryRel := filepath.Join("my mod", "entry.ts")
	entryPath := filepath.Join(dir, entryRel)
	if err := os.WriteFile(entryPath, []byte("import.meta.url"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewPaseratiWithBaseDir(dir)
	p.SetSkipTypeCheck(true)

	val, compileErrs, runtimeErrs := p.RunModuleWithValue("./" + filepath.ToSlash(entryRel))
	if len(compileErrs) > 0 {
		t.Fatalf("compile errors: %v", compileErrs[0])
	}
	if len(runtimeErrs) > 0 {
		t.Fatalf("runtime errors: %v", runtimeErrs[0])
	}

	got := val.ToString()
	if strings.Contains(got, "my mod") {
		t.Errorf("import.meta.url = %q, space in path must be percent-encoded", got)
	}
	if !strings.Contains(got, "%20") {
		t.Errorf("import.meta.url = %q, want %%20 for space in directory name", got)
	}
}
