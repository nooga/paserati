package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/nooga/paserati/pkg/modules"
)

// classicResolver is --moduleResolution classic for a non-relative
// specifier: tsc looks for the module as a file (name.ts, name.tsx,
// name.d.ts) in the importing file's directory and then in each directory
// above it.
type classicResolver struct {
	root string
	fs   *modules.FileSystemResolver
}

func (r *classicResolver) Name() string  { return "TestClassic" }
func (r *classicResolver) Priority() int { return 5 }
func (r *classicResolver) CanResolve(specifier string) bool {
	return specifier != "" && !strings.HasPrefix(specifier, ".") && !strings.HasPrefix(specifier, "/")
}
func (r *classicResolver) Resolve(specifier, fromPath string) (*modules.ResolvedModule, error) {
	for dir := filepath.Dir(fromPath); strings.HasPrefix(dir, r.root); dir = filepath.Dir(dir) {
		if res, err := r.fs.Resolve("./"+specifier, filepath.Join(dir, "importer")); err == nil {
			return res, nil
		}
		if dir == r.root {
			break
		}
	}
	return nil, fmt.Errorf("cannot find module %q", specifier)
}
