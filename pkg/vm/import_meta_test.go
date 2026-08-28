package vm

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestImportMetaURL(t *testing.T) {
	t.Parallel()

	absFoo, err := filepath.Abs("foo.ts")
	if err != nil {
		t.Fatal(err)
	}
	absRelFoo, err := filepath.Abs("./foo.ts")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		input string
		want  string
		skip  string
	}{
		{
			name:  "empty",
			input: "",
			want:  "",
		},
		{
			name:  "unix absolute",
			input: "/tmp/mod.ts",
			want:  "file:///tmp/mod.ts",
			skip:  "windows",
		},
		{
			name:  "space encoding",
			input: "/tmp/my mod.ts",
			want:  "file:///tmp/my%20mod.ts",
			skip:  "windows",
		},
		{
			name:  "native scheme unchanged",
			input: "native://fs",
			want:  "native://fs",
		},
		{
			name:  "file scheme unchanged",
			input: "file:///already",
			want:  "file:///already",
		},
		{
			name:  "opaque code module",
			input: "__code_module__",
			want:  "__code_module__",
		},
		{
			name:  "relative foo.ts",
			input: "foo.ts",
			want:  pathToFileURL(absFoo),
		},
		{
			name:  "relative ./foo.ts",
			input: "./foo.ts",
			want:  pathToFileURL(absRelFoo),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.skip == "windows" && runtime.GOOS == "windows" {
				t.Skip("unix-specific path")
			}
			if got := ImportMetaURL(tt.input); got != tt.want {
				t.Errorf("ImportMetaURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestImportMetaURLWithBase(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	got := ImportMetaURLWithBase("entry.ts", base)
	abs, err := filepath.Abs(filepath.Join(base, "entry.ts"))
	if err != nil {
		t.Fatal(err)
	}
	want := pathToFileURL(abs)
	if got != want {
		t.Errorf("ImportMetaURLWithBase(entry.ts, base) = %q, want %q", got, want)
	}

	absX := "/abs/x.ts"
	if runtime.GOOS == "windows" {
		absX = `C:\abs\x.ts`
	}
	got = ImportMetaURLWithBase(absX, base)
	wantAbs := pathToFileURL(absX)
	if runtime.GOOS != "windows" {
		if a, err := filepath.Abs(absX); err == nil {
			wantAbs = pathToFileURL(a)
		}
	}
	if got != wantAbs {
		t.Errorf("absolute path must ignore baseDir, got %q want %q", got, wantAbs)
	}
}
