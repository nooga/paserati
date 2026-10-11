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
	canon   map[string]canonResult
}

type canonResult struct {
	path string
	err  error
}

func newFSCache() *fsCache {
	return &fsCache{
		dirs:    make(map[string]map[string]fs.FileMode),
		follows: make(map[string]os.FileInfo),
		canon:   make(map[string]canonResult),
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
func (c *fsCache) canonicalPath(path string) (string, error) {
	if c == nil {
		return canonicalPath(path)
	}
	c.mu.Lock()
	res, ok := c.canon[path]
	c.mu.Unlock()
	if ok {
		return res.path, res.err
	}
	p, err := canonicalPath(path)
	c.mu.Lock()
	c.canon[path] = canonResult{p, err}
	c.mu.Unlock()
	return p, err
}
