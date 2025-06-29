package modules

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MemoryResolver resolves modules from an in-memory store
type MemoryResolver struct {
	name     string                   // Human-readable name
	modules  map[string]*MemoryModule // Map of module path -> module
	mutex    sync.RWMutex             // Protects concurrent access
	priority int                      // Resolution priority
}

// MemoryModule represents a module stored in memory
type MemoryModule struct {
	Path     string    // Module path
	Content  string    // Module source content
	Created  time.Time // When the module was created
	Modified time.Time // When the module was last modified
}

// NewMemoryResolver creates a new memory-based module resolver
func NewMemoryResolver(name string) *MemoryResolver {
	if name == "" {
		name = "Memory"
	}
	
	return &MemoryResolver{
		name:     name,
		modules:  make(map[string]*MemoryModule),
		priority: 50, // Higher priority than file system for testing
	}
}

// Name returns the resolver name
func (r *MemoryResolver) Name() string {
	return r.name
}

// CanResolve returns true if this resolver can handle the specifier
func (r *MemoryResolver) CanResolve(specifier string) bool {
	// Use the same logic as findModule to check if we can resolve
	_, _, err := r.findModule(specifier, "")
	return err == nil
}

// Priority returns the resolver priority
func (r *MemoryResolver) Priority() int {
	return r.priority
}

// Resolve resolves a module specifier to a concrete module
func (r *MemoryResolver) Resolve(specifier string, fromPath string) (*ResolvedModule, error) {
	// Calculate the resolved path
	resolvedPath, module, err := r.findModule(specifier, fromPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %w", specifier, err)
	}
	
	// Create a ReadCloser for the module content
	source := &memoryReadCloser{
		reader: strings.NewReader(module.Content),
		module: module,
	}
	
	return &ResolvedModule{
		Specifier:    specifier,
		ResolvedPath: resolvedPath,
		Source:       source,
		FS:           &memoryFS{resolver: r},
		Resolver:     r.name,
	}, nil
}

// findModule finds a module by specifier with various resolution strategies
func (r *MemoryResolver) findModule(specifier string, fromPath string) (string, *MemoryModule, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	// Handle relative paths
	if strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../") {
		if fromPath == "" {
			// If no fromPath provided, treat as relative to current directory
			// Remove the leading "./" for root-relative resolution
			if strings.HasPrefix(specifier, "./") {
				specifier = strings.TrimPrefix(specifier, "./")
			} else {
				return "", nil, fmt.Errorf("relative import %s requires fromPath", specifier)
			}
		} else {
			fromDir := filepath.Dir(fromPath)
			targetPath := filepath.Join(fromDir, specifier)
			specifier = filepath.Clean(targetPath)
		}
	}
	
	// Strategy 1: Try exact path
	if module, exists := r.modules[specifier]; exists {
		return specifier, module, nil
	}
	
	// Strategy 2: Try with extensions
	extensions := []string{".ts", ".tsx", ".js", ".jsx", ".d.ts"}
	for _, ext := range extensions {
		pathWithExt := specifier + ext
		if module, exists := r.modules[pathWithExt]; exists {
			return pathWithExt, module, nil
		}
	}
	
	// Strategy 3: Try as directory with index files
	indexFiles := []string{"index.ts", "index.tsx", "index.js", "index.jsx"}
	for _, indexFile := range indexFiles {
		indexPath := filepath.Join(specifier, indexFile)
		if module, exists := r.modules[indexPath]; exists {
			return indexPath, module, nil
		}
	}
	
	return "", nil, fmt.Errorf("module not found: %s", specifier)
}

// AddModule adds a module to the memory store
func (r *MemoryResolver) AddModule(path string, content string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	now := time.Now()
	r.modules[path] = &MemoryModule{
		Path:     path,
		Content:  content,
		Created:  now,
		Modified: now,
	}
}

// UpdateModule updates an existing module's content
func (r *MemoryResolver) UpdateModule(path string, content string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	module, exists := r.modules[path]
	if !exists {
		return fmt.Errorf("module not found: %s", path)
	}
	
	module.Content = content
	module.Modified = time.Now()
	return nil
}

// RemoveModule removes a module from the memory store
func (r *MemoryResolver) RemoveModule(path string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	delete(r.modules, path)
}

// ListModules returns all module paths in the store
func (r *MemoryResolver) ListModules() []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	paths := make([]string, 0, len(r.modules))
	for path := range r.modules {
		paths = append(paths, path)
	}
	return paths
}

// Clear removes all modules from the store
func (r *MemoryResolver) Clear() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	r.modules = make(map[string]*MemoryModule)
}

// GetModule returns a module by path (for testing/debugging)
func (r *MemoryResolver) GetModule(path string) *MemoryModule {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	return r.modules[path]
}

// SetPriority sets the resolver priority
func (r *MemoryResolver) SetPriority(priority int) {
	r.priority = priority
}

// memoryReadCloser implements io.ReadCloser for memory modules
type memoryReadCloser struct {
	reader io.Reader
	module *MemoryModule
}

func (mrc *memoryReadCloser) Read(p []byte) (n int, err error) {
	return mrc.reader.Read(p)
}

func (mrc *memoryReadCloser) Close() error {
	// Nothing to close for memory reader
	return nil
}

// memoryFile implements fs.File for memory modules
type memoryFile struct {
	name   string
	reader io.Reader
	module *MemoryModule
	closed bool
}

func (mf *memoryFile) Stat() (fs.FileInfo, error) {
	return &memoryFileInfo{
		name:    filepath.Base(mf.name),
		size:    int64(len(mf.module.Content)),
		modTime: mf.module.Modified,
	}, nil
}

func (mf *memoryFile) Read(p []byte) (int, error) {
	if mf.closed {
		return 0, fmt.Errorf("file is closed")
	}
	return mf.reader.Read(p)
}

func (mf *memoryFile) Close() error {
	mf.closed = true
	return nil
}

// memoryFileInfo implements fs.FileInfo for memory files
type memoryFileInfo struct {
	name    string
	size    int64
	modTime time.Time
}

func (mfi *memoryFileInfo) Name() string {
	return mfi.name
}

func (mfi *memoryFileInfo) Size() int64 {
	return mfi.size
}

func (mfi *memoryFileInfo) Mode() fs.FileMode {
	return 0644 // Regular file with read/write permissions
}

func (mfi *memoryFileInfo) ModTime() time.Time {
	return mfi.modTime
}

func (mfi *memoryFileInfo) IsDir() bool {
	return false
}

func (mfi *memoryFileInfo) Sys() interface{} {
	return nil
}

// memoryFS implements ModuleFS for memory resolver
type memoryFS struct {
	resolver *MemoryResolver
}

func (mfs *memoryFS) Open(name string) (fs.File, error) {
	mfs.resolver.mutex.RLock()
	defer mfs.resolver.mutex.RUnlock()
	
	module, exists := mfs.resolver.modules[name]
	if !exists {
		return nil, fmt.Errorf("file not found: %s", name)
	}
	
	return &memoryFile{
		name:   name,
		reader: strings.NewReader(module.Content),
		module: module,
	}, nil
}

func (mfs *memoryFS) ReadFile(name string) ([]byte, error) {
	mfs.resolver.mutex.RLock()
	defer mfs.resolver.mutex.RUnlock()
	
	module, exists := mfs.resolver.modules[name]
	if !exists {
		return nil, fmt.Errorf("file not found: %s", name)
	}
	
	return []byte(module.Content), nil
}

// AddTestModules is a helper function to add common test modules
func (r *MemoryResolver) AddTestModules() {
	// Add some common test modules
	r.AddModule("test/module-a.ts", `
export function greet(name: string): string {
    return "Hello, " + name + "!";
}

export const VERSION = "1.0.0";
`)
	
	r.AddModule("test/module-b.ts", `
import { greet, VERSION } from "./module-a";

export function welcome(name: string): string {
    return greet(name) + " Version: " + VERSION;
}
`)
	
	r.AddModule("test/utils/index.ts", `
export * from "./helper";
export { default as config } from "./config";
`)
	
	r.AddModule("test/utils/helper.ts", `
export function isString(value: any): value is string {
    return typeof value === "string";
}

export function isNumber(value: any): value is number {
    return typeof value === "number";
}
`)
	
	r.AddModule("test/utils/config.ts", `
export default {
    debug: false,
    apiUrl: "https://api.example.com",
    timeout: 5000,
};
`)
}