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
	dirs    map[string]map[string]fs.FileMode
	follows map[string]os.FileInfo
	canon   map[string]string
}

func newFSCache() *fsCache {
	return &fsCache{
		dirs:    make(map[string]map[string]fs.FileMode),
		follows: make(map[string]os.FileInfo),
		canon:   make(map[string]string),
	}
}

// stat reports whether path exists and is a directory, following symlinks.
func (c *fsCache) stat(path string) (exists, dir bool) {
	if c == nil {
		info, err := os.Stat(path)
		return err == nil, err == nil && info.IsDir()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	path = filepath.Clean(path)
	parent, name := filepath.Split(path)
	if name == "" {
		// A root ("/") has no parent listing to look in.
		info, err := os.Stat(path)
		return err == nil, err == nil && info.IsDir()
	}
	parent = filepath.Clean(parent)
	entries, ok := c.dirs[parent]
	if !ok {
		list, _ := os.ReadDir(parent)
		entries = make(map[string]fs.FileMode, len(list))
		for _, e := range list {
			entries[e.Name()] = e.Type()
		}
		c.dirs[parent] = entries
	}
	mode, ok := entries[name]
	if !ok {
		return false, false
	}
	if mode&fs.ModeSymlink == 0 {
		return true, mode.IsDir()
	}
	info, seen := c.follows[path]
	if !seen {
		info, _ = os.Stat(path)
		c.follows[path] = info
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
		c.stat(abs) // fills the parent's listing
		c.mu.Lock()
		mode, listed := c.dirs[parent][name]
		c.mu.Unlock()
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
