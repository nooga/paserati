package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nooga/paserati/pkg/lexer"
)

// TestParseTypeScriptLibFiles parses every lib/*.d.ts of a TypeScript checkout
// (set TS_REPO to the repo root) and requires zero syntax errors.
func TestParseTypeScriptLibFiles(t *testing.T) {
	repo := os.Getenv("TS_REPO")
	if repo == "" {
		t.Skip("TS_REPO not set")
	}
	files, _ := filepath.Glob(filepath.Join(repo, "src/lib/*.d.ts"))
	if len(files) == 0 {
		t.Fatalf("no lib files found under %s", repo)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		p := NewParser(lexer.NewLexer(string(data)))
		_, errs := p.ParseProgram()
		if len(errs) > 0 {
			n := len(errs)
			if n > 3 {
				n = 3
			}
			t.Errorf("%s: %d errors, first: %v", filepath.Base(f), len(errs), errs[:n])
		}
	}
}
