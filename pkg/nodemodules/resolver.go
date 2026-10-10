package nodemodules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nooga/paserati/pkg/modules"
)

// Resolver is a modules.ModuleResolver for bare package specifiers. Register
// it with Paserati.AddResolver.
type Resolver struct {
	Options
	priority int
}

// NewResolver returns a resolver for the given options. It runs after native
// modules (-100), so a host's own bare names win, and before the file-system
// resolver (100), which would otherwise look for "zod" as ./zod.ts.
func NewResolver(opts Options) *Resolver {
	return &Resolver{Options: opts, priority: 50}
}

func (r *Resolver) Name() string { return "NodeModules" }

func (r *Resolver) Priority() int { return r.priority }

// CanResolve claims bare specifiers, and in Types mode relative ones too, so
// "./x.js" finds x.ts or x.d.ts before the file-system resolver finds x.js.
func (r *Resolver) CanResolve(specifier string) bool {
	if r.Mode == Types && isRelative(specifier) {
		return true
	}
	return IsBareSpecifier(specifier)
}

func (r *Resolver) Resolve(specifier, fromPath string) (*modules.ResolvedModule, error) {
	var path string
	var err error
	if isRelative(specifier) {
		path, err = ResolveRelative(specifier, startDir(fromPath), r.Options)
	} else {
		path, err = Resolve(specifier, startDir(fromPath), r.Options)
	}
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open %q: %w", path, err)
	}
	return &modules.ResolvedModule{
		Specifier:    specifier,
		ResolvedPath: path,
		Source:       f,
		Resolver:     r.Name(),
	}, nil
}

// startDir is the directory an import in fromPath resolves from: the file's
// directory, or the working directory for a script without a path.
func startDir(fromPath string) string {
	if fromPath == "" || fromPath == "." || fromPath == "<eval>" {
		if wd, err := os.Getwd(); err == nil {
			return wd
		}
		return "."
	}
	if isDir(fromPath) {
		return fromPath
	}
	return filepath.Dir(fromPath)
}

func isRelative(specifier string) bool {
	return specifier == "." || specifier == ".." ||
		strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../")
}
