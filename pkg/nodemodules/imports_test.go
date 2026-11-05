package nodemodules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A package resolves its own name through "exports", and "#name" through
// "imports"; a .cjs target stands for a .cts file in Types mode.
func TestSelfNameAndImports(t *testing.T) {
	d := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(d, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("package.json", `{"name":"package","exports":"./index.js","imports":{"#cjs":"./index.cjs","#type":"./index.js"}}`)
	write("index.ts", "export const a = 1")
	write("index.cts", "export const a = 1")
	for spec, want := range map[string]string{"package": "index.ts", "#cjs": "index.cts", "#type": "index.ts"} {
		got, err := Resolve(spec, d, Options{Mode: Types, fs: newFSCache()})
		if err != nil || !strings.HasSuffix(got, want) {
			t.Errorf("Resolve(%q) = %q, %v; want %s", spec, got, err, want)
		}
	}
}
