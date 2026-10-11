package tsconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, content := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStripJSONC(t *testing.T) {
	src := `{
  // a comment
  "a": "x // not a comment", /* block */
  "b": [1, 2,],
  "c": { "d": true, },
}`
	var got map[string]any
	if err := json.Unmarshal(StripJSONC([]byte(src)), &got); err != nil {
		t.Fatalf("not JSON after stripping: %v", err)
	}
	want := map[string]any{"a": "x // not a comment", "b": []any{1.0, 2.0}, "c": map[string]any{"d": true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLoadProject(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeFiles(t, root, map[string]string{
		"tsconfig.base.json": `{ "compilerOptions": { "strict": true, "noImplicitOverride": true, "paths": { "@app/*": ["./lib/*"] } }, "exclude": ["**/*.test.ts"] }`,
		"tsconfig.json": `{
  // the root config extends the base and narrows include
  "extends": "./tsconfig.base.json",
  "include": ["src", "types/*.d.ts"],
  "compilerOptions": { "strict": false, "strictNullChecks": true, },
}`,
		"src/a.ts":                            "",
		"src/a.test.ts":                       "",
		"src/deep/b.tsx":                      "",
		"src/c.js":                            "",
		"src/node_modules/x/index.ts":         "",
		"types/g.d.ts":                        "",
		"other/d.ts":                          "",
		"lib/x.ts":                            "",
		"node_modules/@types/node/index.d.ts": "",
		"node_modules/@types/scope__pkg/index.d.ts": "",
	})
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var rel []string
	for _, f := range cfg.Files {
		r, _ := filepath.Rel(root, f)
		rel = append(rel, filepath.ToSlash(r))
	}
	if want := []string{"src/a.ts", "src/deep/b.tsx", "types/g.d.ts"}; !reflect.DeepEqual(rel, want) {
		t.Errorf("files = %v, want %v", rel, want)
	}
	if cfg.StrictFamily("noImplicitAny") {
		t.Errorf("noImplicitAny: strict is overridden to false by the child")
	}
	if !cfg.StrictFamily("strictNullChecks") {
		t.Errorf("strictNullChecks is set explicitly")
	}
	if v, _ := cfg.Bool("noImplicitOverride"); !v {
		t.Errorf("noImplicitOverride from the base is lost")
	}
	if len(cfg.Paths) != 1 || cfg.Paths[0].Targets[0] != filepath.Join(root, "lib", "*") {
		t.Errorf("paths = %+v, want @app/* -> <root>/lib/*", cfg.Paths)
	}
	if got, want := cfg.Types(), []string{"@scope/pkg", "node"}; !reflect.DeepEqual(got, want) {
		t.Errorf("types = %v, want %v", got, want)
	}
}

func TestDefaultIncludeAndExplicitTypes(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeFiles(t, root, map[string]string{
		"tsconfig.json":         `{ "compilerOptions": { "types": ["vitest/globals"], "outDir": "dist" } }`,
		"a.ts":                  "",
		"dist/a.d.ts":           "",
		"node_modules/p/i.d.ts": "",
	})
	cfg, err := Load(filepath.Join(root, "tsconfig.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Files) != 1 || filepath.Base(cfg.Files[0]) != "a.ts" {
		t.Errorf("files = %v, want [a.ts] (outDir and node_modules excluded)", cfg.Files)
	}
	if got := cfg.Types(); !reflect.DeepEqual(got, []string{"vitest/globals"}) {
		t.Errorf("types = %v", got)
	}
	if !cfg.StrictFamily("strictNullChecks") {
		t.Errorf("strict family defaults to on")
	}
}
