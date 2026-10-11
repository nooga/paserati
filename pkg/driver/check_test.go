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

// TestCheckFilesGlobalDeclarations checks @types/node's web-globals pattern:
// a module-local interface feeds a `declare global` interface through a
// `typeof globalThis` conditional, and a global extended by one file
// (EventTarget) is declared by a file loaded after it.
func TestCheckFilesGlobalDeclarations(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"node_modules/@types/web/package.json": `{"types":"index.d.ts"}`,
		"node_modules/@types/web/index.d.ts":   "/// <reference path=\"abort.d.ts\" />\n/// <reference path=\"events.d.ts\" />\n",
		"node_modules/@types/web/abort.d.ts": "export {};\n" +
			"type _AbortSignal = typeof globalThis extends { onmessage: any } ? {} : AbortSignal;\n" +
			"interface AbortSignal extends EventTarget { readonly aborted: boolean; }\n" +
			"declare global { interface AbortSignal extends _AbortSignal {} }\n",
		"node_modules/@types/web/events.d.ts": "export {};\n" +
			"type _EventTarget = typeof globalThis extends { onmessage: any } ? {} : EventTarget;\n" +
			"interface EventTarget { addEventListener(type: string, listener: () => void): void; }\n" +
			"declare global { interface EventTarget extends _EventTarget {} }\n",
		"main.ts": "declare const s: AbortSignal;\n" +
			"s.addEventListener(\"abort\", () => {});\n" +
			"const a: boolean = s.aborted;\n" +
			"const bad: number = s.aborted;\n" +
			"export {};\n",
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
	opts := DefaultCheckOptions()
	opts.Types = []string{"web"}
	opts.TypesFrom = root
	var got []string
	for _, r := range p.CheckFilesWithOptions([]string{"main.ts"}, opts) {
		for _, d := range r.Diagnostics {
			got = append(got, d.Code()+" "+d.Message())
		}
	}
	want := "TS2322 Type 'boolean' is not assignable to type 'number'."
	if len(got) != 1 || got[0] != want {
		t.Errorf("diagnostics = %q, want only %q", got, want)
	}
}
