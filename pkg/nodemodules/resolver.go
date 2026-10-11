package nodemodules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nooga/paserati/pkg/modules"
)

// Resolver is a modules.ModuleResolver for bare package specifiers. Register
// it with Paserati.AddResolver.
type Resolver struct {
	Options
	priority int

	// In Types mode the file system is taken to hold still for the whole
	// check, as tsc's module resolution cache does: a specifier resolves once
	// per directory, and an importer's directory is found once.
	mu       sync.Mutex
	resolved map[resolveKey]resolveResult
	dirs     map[string]string
}

type resolveKey struct{ specifier, dir string }

type resolveResult struct {
	path string
	err  error
}

// NewResolver returns a resolver for the given options. It runs after native
// modules (-100), so a host's own bare names win, and before the file-system
// resolver (100), which would otherwise look for "zod" as ./zod.ts.
func NewResolver(opts Options) *Resolver {
	if opts.Mode == Types {
		opts.fs = newFSCache()
	}
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
	path, err := r.resolvePath(specifier, fromPath)
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

func (r *Resolver) resolvePath(specifier, fromPath string) (string, error) {
	if r.Mode != Types {
		return resolvePath(specifier, startDir(nil, fromPath), r.Options)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	dir, ok := r.dirs[fromPath]
	if !ok {
		dir = startDir(r.fs, fromPath)
		if r.dirs == nil {
			r.dirs = make(map[string]string)
			r.resolved = make(map[resolveKey]resolveResult)
		}
		r.dirs[fromPath] = dir
	}
	key := resolveKey{specifier, dir}
	if res, ok := r.resolved[key]; ok {
		return res.path, res.err
	}
	path, err := resolvePath(specifier, dir, r.Options)
	r.resolved[key] = resolveResult{path, err}
	return path, err
}

func resolvePath(specifier, dir string, opts Options) (string, error) {
	if isRelative(specifier) {
		return ResolveRelative(specifier, dir, opts)
	}
	return Resolve(specifier, dir, opts)
}

// startDir is the directory an import in fromPath resolves from: the file's
// directory, or the working directory for a script without a path.
func startDir(c *fsCache, fromPath string) string {
	if fromPath == "" || fromPath == "." || fromPath == "<eval>" {
		if wd, err := os.Getwd(); err == nil {
			return wd
		}
		return "."
	}
	if c.isDir(fromPath) {
		return fromPath
	}
	return filepath.Dir(fromPath)
}

func isRelative(specifier string) bool {
	return specifier == "." || specifier == ".." ||
		strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../")
}
