package nodemodules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTree writes files (path -> content) under root.
func writeTree(t *testing.T, root string, files map[string]string) {
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

func TestResolve(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeTree(t, root, map[string]string{
		// zod 3 shape: a "types" condition inside the "." conditions.
		"node_modules/zod/package.json": `{"types":"./index.d.cts","exports":{".":{"types":"./index.d.cts","import":"./index.js","require":"./index.cjs"},"./package.json":"./package.json"}}`,
		"node_modules/zod/index.d.cts":  ``,
		"node_modules/zod/index.js":     ``,
		"node_modules/zod/index.cjs":    ``,
		// No exports; "typings" field and "main".
		"node_modules/old/package.json":  `{"main":"lib/main.js","typings":"lib/main.d.ts"}`,
		"node_modules/old/lib/main.js":   ``,
		"node_modules/old/lib/main.d.ts": ``,
		"node_modules/old/lib/util.js":   ``,
		"node_modules/old/lib/util.d.ts": ``,
		// Untyped package with DefinitelyTyped types, scoped.
		"node_modules/@babel/core/package.json":        `{"main":"lib/index.js"}`,
		"node_modules/@babel/core/lib/index.js":        ``,
		"node_modules/@types/babel__core/package.json": `{"types":"index.d.ts"}`,
		"node_modules/@types/babel__core/index.d.ts":   ``,
		// Subpath patterns and a declaration next to the JS target.
		"node_modules/pat/package.json":               `{"exports":{".":"./dist/index.js","./*":"./dist/*.js","./features/*":"./dist/features/*/index.js","./internal/*":null}}`,
		"node_modules/pat/dist/index.js":              ``,
		"node_modules/pat/dist/index.d.ts":            ``,
		"node_modules/pat/dist/a.js":                  ``,
		"node_modules/pat/dist/a.d.ts":                ``,
		"node_modules/pat/dist/features/x/index.js":   ``,
		"node_modules/pat/dist/features/x/index.d.ts": ``,
		// Conditions object without "." keys, require vs import.
		"node_modules/cond/package.json": `{"exports":{"require":"./c.cjs","import":"./m.mjs"}}`,
		"node_modules/cond/c.cjs":        ``,
		"node_modules/cond/m.mjs":        ``,
		"node_modules/cond/m.d.mts":      ``,
		// A nested project resolving up to the root node_modules.
		"app/src/main.ts": ``,
	})
	nm := filepath.Join(root, "node_modules")
	from := filepath.Join(root, "app", "src")

	cases := []struct {
		spec string
		opts Options
		want string // relative to node_modules; "" = must fail
	}{
		{"zod", Options{Mode: Types}, "zod/index.d.cts"},
		{"zod", Options{Mode: Runtime}, "zod/index.js"},
		{"zod", Options{Mode: Runtime, Require: true}, "zod/index.cjs"},
		{"zod/package.json", Options{Mode: Runtime}, "zod/package.json"},
		{"zod/lib/hidden", Options{Mode: Types}, ""},
		{"old", Options{Mode: Types}, "old/lib/main.d.ts"},
		{"old", Options{Mode: Runtime}, "old/lib/main.js"},
		{"old/lib/util", Options{Mode: Types}, "old/lib/util.d.ts"},
		{"old/lib/util.js", Options{Mode: Types}, "old/lib/util.d.ts"},
		{"@babel/core", Options{Mode: Types}, "@types/babel__core/index.d.ts"},
		{"@babel/core", Options{Mode: Runtime}, "@babel/core/lib/index.js"},
		{"pat", Options{Mode: Types}, "pat/dist/index.d.ts"},
		{"pat/a", Options{Mode: Types}, "pat/dist/a.d.ts"},
		{"pat/a", Options{Mode: Runtime}, "pat/dist/a.js"},
		{"pat/features/x", Options{Mode: Types}, "pat/dist/features/x/index.d.ts"},
		{"pat/internal/y", Options{Mode: Runtime}, ""},
		{"cond", Options{Mode: Types}, "cond/m.d.mts"},
		{"cond", Options{Mode: Runtime, Require: true}, "cond/c.cjs"},
		{"missing", Options{Mode: Types}, ""},
	}
	for _, c := range cases {
		got, err := Resolve(c.spec, from, c.opts)
		if c.want == "" {
			if err == nil {
				t.Errorf("Resolve(%q, mode %d) = %s, want an error", c.spec, c.opts.Mode, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Resolve(%q, mode %d): %v", c.spec, c.opts.Mode, err)
			continue
		}
		if want := filepath.Join(nm, filepath.FromSlash(c.want)); got != want {
			t.Errorf("Resolve(%q, mode %d) = %s, want %s", c.spec, c.opts.Mode, strings.TrimPrefix(got, nm), c.want)
		}
	}
}

func TestSplitSpecifier(t *testing.T) {
	for spec, want := range map[string][2]string{
		"zod":         {"zod", ""},
		"zod/v4/core": {"zod", "v4/core"},
		"@a/b":        {"@a/b", ""},
		"@a/b/c/d":    {"@a/b", "c/d"},
		"@scope-only": {"", ""},
	} {
		if n, s := SplitSpecifier(spec); n != want[0] || s != want[1] {
			t.Errorf("SplitSpecifier(%q) = %q, %q; want %q, %q", spec, n, s, want[0], want[1])
		}
	}
}

func TestResolvePaths(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"packages/client/src/index.ts", "packages/client/src/links/http.ts", "types/exact.d.ts"} {
		full := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paths := []PathMapping{
		{Pattern: "@trpc/client", Targets: []string{filepath.Join(root, "packages/client/src")}},
		{Pattern: "@trpc/client/*", Targets: []string{filepath.Join(root, "packages/client/src/*")}},
		{Pattern: "exact", Targets: []string{filepath.Join(root, "missing"), filepath.Join(root, "types/exact")}},
	}
	cases := map[string]string{
		"@trpc/client":            "packages/client/src/index.ts",
		"@trpc/client/links/http": "packages/client/src/links/http.ts",
		"exact":                   "types/exact.d.ts",
	}
	for spec, want := range cases {
		got, ok, err := ResolvePaths(spec, paths, Types)
		if !ok || err != nil || got != filepath.Join(root, filepath.FromSlash(want)) {
			t.Errorf("%s: got %q ok=%v err=%v, want %s", spec, got, ok, err, want)
		}
	}
	if _, ok, _ := ResolvePaths("zod", paths, Types); ok {
		t.Errorf("zod matched a paths pattern")
	}
}
