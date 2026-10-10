package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nooga/paserati/pkg/nodemodules"
)

func TestCheckFiles(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		// A typed package: its types reach the checker, its body never runs.
		"node_modules/pkg/package.json": `{"exports":{".":{"types":"./index.d.ts","import":"./index.js"}}}`,
		"node_modules/pkg/index.d.ts":   "export declare function twice(n: number): number;\nexport declare const broken: Missing;\n",
		"node_modules/pkg/index.js":     "throw new Error('must not run');\n",
		"lib.ts":                        "export const base: number = 1;\n",
		"main.ts":                       "import { twice } from \"pkg\";\nimport { base } from \"./lib.ts\";\nconst s: string = twice(base);\n",
	}
	for p, content := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	p := NewPaserati()
	p.AddResolver(nodemodules.NewResolver(nodemodules.Options{Mode: nodemodules.Types}))
	results := p.CheckFiles([]string{"main.ts"})

	byFile := map[string][]string{}
	for _, r := range results {
		if r.LoadError != nil {
			t.Fatalf("%s: load error: %v", r.Path, r.LoadError)
		}
		rel, _ := filepath.Rel(root, r.Path)
		for _, d := range r.Diagnostics {
			byFile[rel] = append(byFile[rel], d.Code()+" "+d.Message())
		}
		if _, ok := byFile[rel]; !ok {
			byFile[rel] = nil
		}
	}
	if _, ok := byFile["lib.ts"]; !ok {
		t.Errorf("lib.ts (imported) was not checked: %v", byFile)
	}
	for f := range byFile {
		if strings.Contains(f, "node_modules") {
			t.Errorf("library file %s reported (skipLibCheck)", f)
		}
	}
	got := byFile["main.ts"]
	if len(got) != 1 || !strings.Contains(got[0], "TS2322") || !strings.Contains(got[0], "'number' is not assignable to type 'string'") {
		t.Errorf("main.ts diagnostics = %q, want one TS2322 number-to-string", got)
	}
}

func TestCheckFilesReportsSyntaxErrors(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bad.ts"), []byte("const x = ;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	results := NewPaserati().CheckFiles([]string{"bad.ts"})
	var got []string
	for _, r := range results {
		for _, d := range r.Diagnostics {
			got = append(got, d.Code())
		}
	}
	if len(got) != 1 || got[0] != "TS1109" {
		t.Errorf("diagnostics = %v, want [TS1109]", got)
	}
}
