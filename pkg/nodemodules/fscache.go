package nodemodules

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// fsCache answers the resolver's file-system questions from directory
// listings read once each, for a resolution pass (a type check) that takes the
// file system to hold still. A relative import tries up to a dozen candidate
// names; one ReadDir of the directory answers all of them, and every later
// import from the same directory. A nil *fsCache asks the file system directly.
type fsCache struct {
	mu      sync.Mutex
	dirs    map[string]*dirListing
	follows map[string]os.FileInfo
	canon   map[string]string
}

// dirListing is one directory's entries; entries is set before ready closes.
// The directory is read outside the cache's lock, once.
type dirListing struct {
	ready   chan struct{}
	entries map[string]fs.FileMode
}

func newFSCache() *fsCache {
	return &fsCache{
		dirs:    make(map[string]*dirListing),
		follows: make(map[string]os.FileInfo),
		canon:   make(map[string]string),
	}
}

// listing returns the entries of the directory dir (a clean path), reading it
// on first use.
func (c *fsCache) listing(dir string) map[string]fs.FileMode {
	c.mu.Lock()
	l, ok := c.dirs[dir]
	if !ok {
		l = &dirListing{ready: make(chan struct{})}
		c.dirs[dir] = l
	}
	c.mu.Unlock()
	if ok {
		<-l.ready
		return l.entries
	}
	list, _ := os.ReadDir(dir)
	l.entries = make(map[string]fs.FileMode, len(list))
	for _, e := range list {
		l.entries[e.Name()] = e.Type()
	}
	close(l.ready)
	return l.entries
}

// stat reports whether path exists and is a directory, following symlinks.
func (c *fsCache) stat(path string) (exists, dir bool) {
	if c == nil {
		info, err := os.Stat(path)
		return err == nil, err == nil && info.IsDir()
	}
	path = filepath.Clean(path)
	parent, name := filepath.Split(path)
	if name == "" {
		// A root ("/") has no parent listing to look in.
		info, err := os.Stat(path)
		return err == nil, err == nil && info.IsDir()
	}
	mode, ok := c.listing(filepath.Clean(parent))[name]
	if !ok {
		return false, false
	}
	if mode&fs.ModeSymlink == 0 {
		return true, mode.IsDir()
	}
	c.mu.Lock()
	info, seen := c.follows[path]
	c.mu.Unlock()
	if !seen {
		info, _ = os.Stat(path)
		c.mu.Lock()
		c.follows[path] = info
		c.mu.Unlock()
	}
	return info != nil, info != nil && info.IsDir()
}

func (c *fsCache) isDir(path string) bool {
	exists, dir := c.stat(path)
	return exists && dir
}

func (c *fsCache) isFile(path string) bool {
	exists, dir := c.stat(path)
	return exists && !dir
}

// canonicalPath makes path absolute with symlinks followed, so one file
// reached through two node_modules paths (pnpm, self-links) is one module.
// Each directory's canonical form is worked out once, so a path costs one
// listing lookup instead of an lstat per component.
func (c *fsCache) canonicalPath(path string) (string, error) {
	if c == nil {
		return canonicalPath(path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return canonicalPath(path)
	}
	return c.canonical(abs), nil
}

func (c *fsCache) canonical(abs string) string {
	c.mu.Lock()
	out, ok := c.canon[abs]
	c.mu.Unlock()
	if ok {
		return out
	}
	parent, name := filepath.Split(abs)
	if name == "" {
		out = abs
	} else {
		parent = filepath.Clean(parent)
		mode, listed := c.listing(parent)[name]
		if listed && mode&fs.ModeSymlink == 0 {
			out = filepath.Join(c.canonical(parent), name)
		} else {
			// A symlink, or a name the listing does not hold: let the OS
			// work it out.
			out, _ = canonicalPath(abs)
		}
	}
	c.mu.Lock()
	c.canon[abs] = out
	c.mu.Unlock()
	return out
}
