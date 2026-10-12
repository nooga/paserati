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
	// per directory, and an importer's directory is found once. Resolutions
	// run outside mu; a caller asking for one in flight waits for it.
	mu       sync.Mutex
	resolved map[resolveKey]*resolveCall
	dirs     map[string]string
}

type resolveKey struct{ specifier, dir string }

// resolveCall is one resolution; path and err are set before done closes.
type resolveCall struct {
	done chan struct{}
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

// ConcurrentSafe reports that Resolve may run on several goroutines at once.
func (r *Resolver) ConcurrentSafe() bool { return true }

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
	// Many resolutions land on a module that is already loaded, and the
	// loader closes the source unread; open the file only once it is read.
	return &modules.ResolvedModule{
		Specifier:    specifier,
		ResolvedPath: path,
		Source:       &lazyFile{path: path},
		Resolver:     r.Name(),
	}, nil
}

func (r *Resolver) resolvePath(specifier, fromPath string) (string, error) {
	if r.Mode != Types {
		return resolvePath(specifier, startDir(nil, fromPath), r.Options)
	}
	r.mu.Lock()
	dir, ok := r.dirs[fromPath]
	r.mu.Unlock()
	if !ok {
		dir = startDir(r.fs, fromPath)
	}
	key := resolveKey{specifier, dir}
	r.mu.Lock()
	if r.dirs == nil {
		r.dirs = make(map[string]string)
		r.resolved = make(map[resolveKey]*resolveCall)
	}
	r.dirs[fromPath] = dir
	call, ok := r.resolved[key]
	if !ok {
		call = &resolveCall{done: make(chan struct{})}
		r.resolved[key] = call
	}
	r.mu.Unlock()
	if ok {
		<-call.done
		return call.path, call.err
	}
	call.path, call.err = resolvePath(specifier, dir, r.Options)
	close(call.done)
	return call.path, call.err
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

// lazyFile opens path on the first Read.
type lazyFile struct {
	path string
	f    *os.File
}

func (l *lazyFile) Read(p []byte) (int, error) {
	if l.f == nil {
		f, err := os.Open(l.path)
		if err != nil {
			return 0, fmt.Errorf("failed to open %q: %w", l.path, err)
		}
		l.f = f
	}
	return l.f.Read(p)
}

func (l *lazyFile) Close() error {
	if l.f == nil {
		return nil
	}
	return l.f.Close()
}
