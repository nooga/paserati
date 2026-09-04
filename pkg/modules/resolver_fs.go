package modules

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

// FileSystemResolver resolves modules from the file system
type FileSystemResolver struct {
	name     string   // Human-readable name
	fs       ModuleFS // File system to resolve from
	priority int      // Resolution priority

	// Configuration
	extensions []string // File extensions to try (e.g., ".ts", ".js", ".d.ts")
	indexFiles []string // Index file names to try (e.g., "index.ts", "index.js")
	baseDir    string   // Base directory for resolution
}

// NewFileSystemResolver creates a new file system resolver
func NewFileSystemResolver(filesystem fs.FS, baseDir string) *FileSystemResolver {
	var moduleFS ModuleFS

	// Wrap the fs.FS to implement ModuleFS if needed
	if mfs, ok := filesystem.(ModuleFS); ok {
		moduleFS = mfs
	} else {
		moduleFS = &fsWrapper{filesystem}
	}

	return &FileSystemResolver{
		name:       "FileSystem",
		fs:         moduleFS,
		priority:   100, // Lower priority than specialized resolvers
		extensions: []string{".ts", ".tsx", ".js", ".jsx", ".d.ts", ".json"},
		indexFiles: []string{"index.ts", "index.tsx", "index.js", "index.jsx"},
		baseDir:    baseDir,
	}
}

// NewOSFileSystemResolver creates a resolver that uses the OS file system
func NewOSFileSystemResolver(baseDir string) *FileSystemResolver {
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		absBaseDir = baseDir
	}

	return &FileSystemResolver{
		name:       "OSFileSystem",
		fs:         &osFS{baseDir: absBaseDir},
		priority:   100,
		extensions: []string{".ts", ".tsx", ".js", ".jsx", ".d.ts", ".json"},
		indexFiles: []string{"index.ts", "index.tsx", "index.js", "index.jsx"},
		baseDir:    absBaseDir,
	}
}

// Name returns the resolver name
func (r *FileSystemResolver) Name() string {
	return r.name
}

// CanResolve returns true if this resolver can handle the specifier
func (r *FileSystemResolver) CanResolve(specifier string) bool {
	// Handle relative and absolute paths
	return strings.HasPrefix(specifier, "./") ||
		strings.HasPrefix(specifier, "../") ||
		strings.HasPrefix(specifier, "/") ||
		filepath.IsAbs(specifier)
}

// Priority returns the resolver priority
func (r *FileSystemResolver) Priority() int {
	return r.priority
}

// Resolve resolves a module specifier to a concrete module
func (r *FileSystemResolver) Resolve(specifier string, fromPath string) (*ResolvedModule, error) {
	// Calculate the target path
	targetPath, err := r.calculateTargetPath(specifier, fromPath)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate target path: %w", err)
	}

	// Try to resolve the exact path
	resolvedPath, err := r.tryResolve(targetPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %w", specifier, err)
	}

	// Open the resolved file
	source, err := r.openFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", resolvedPath, err)
	}

	return &ResolvedModule{
		Specifier:    specifier,
		ResolvedPath: resolvedPath,
		Source:       source,
		FS:           r.fs,
		Resolver:     r.name,
	}, nil
}

// calculateTargetPath calculates the target path from specifier and fromPath
func (r *FileSystemResolver) calculateTargetPath(specifier string, fromPath string) (string, error) {
	if strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../") {
		// Relative path resolution
		if fromPath == "" {
			// If no fromPath provided, treat as relative to current directory
			// Remove the leading "./" for root-relative resolution
			if strings.HasPrefix(specifier, "./") {
				return strings.TrimPrefix(specifier, "./"), nil
			}
			return "", fmt.Errorf("relative import %s requires fromPath", specifier)
		}

		if filepath.IsAbs(fromPath) {
			// fromPath is a real OS filesystem path here (e.g. the CLI's
			// entry file, given by absolute path - #232), not a "/"-
			// separated module specifier. On Windows that means backslashes
			// and a drive letter, which pathpkg (forward-slash only) can't
			// parse: pathpkg.Dir on "C:\Users\...\entry.mjs" sees no "/" at
			// all and returns ".", silently losing the entry file's real
			// directory - the exact same "no resolver could handle
			// specifier" failure #232 already had, just Windows-only and
			// unfixed by switching resolvers alone. filepath.Join also
			// accepts (and correctly normalizes) a forward-slash specifier
			// like "./helper.mjs" here, so this only needs to swap which
			// path package does the joining, not touch the specifier.
			return filepath.Join(filepath.Dir(fromPath), specifier), nil
		}
		fromDir := pathpkg.Dir(fromPath)
		return pathpkg.Join(fromDir, specifier), nil
	}

	if strings.HasPrefix(specifier, "/") || filepath.IsAbs(specifier) {
		// Absolute path
		if strings.HasPrefix(specifier, "/") && r.baseDir != "" {
			// Make relative to base directory
			return strings.TrimPrefix(specifier, "/"), nil
		}
		return specifier, nil
	}

	return "", fmt.Errorf("unsupported specifier format: %s", specifier)
}

// tryResolve attempts to resolve a path with various strategies
func (r *FileSystemResolver) tryResolve(targetPath string) (string, error) {
	// Clean the path
	targetPath = pathpkg.Clean(targetPath)

	// Strategy 1: Try exact path (must be a file, not directory)
	if r.isFile(targetPath) {
		return targetPath, nil
	}

	// Strategy 1.5: Handle .js → .ts mapping for TypeScript projects
	// If the requested file ends with .js but doesn't exist, try .ts
	if strings.HasSuffix(targetPath, ".js") {
		tsPath := strings.TrimSuffix(targetPath, ".js") + ".ts"
		if r.isFile(tsPath) {
			return tsPath, nil
		}
		// Also try .tsx as a fallback
		tsxPath := strings.TrimSuffix(targetPath, ".js") + ".tsx"
		if r.isFile(tsxPath) {
			return tsxPath, nil
		}
	}

	// Strategy 2: Try with extensions
	for _, ext := range r.extensions {
		pathWithExt := targetPath + ext
		if r.isFile(pathWithExt) {
			return pathWithExt, nil
		}
	}

	// Strategy 3: Try as directory with index files
	for _, indexFile := range r.indexFiles {
		indexPath := pathpkg.Join(targetPath, indexFile)
		if r.isFile(indexPath) {
			return indexPath, nil
		}
	}

	return "", fmt.Errorf("module not found: %s", targetPath)
}

// fileExists checks if a file exists in the file system
func (r *FileSystemResolver) fileExists(path string) bool {
	_, err := fs.Stat(r.fs, path)
	return err == nil
}

// isFile checks if a path exists and is a file (not a directory)
func (r *FileSystemResolver) isFile(path string) bool {
	info, err := fs.Stat(r.fs, path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// openFile opens a file and returns a ReadCloser
func (r *FileSystemResolver) openFile(path string) (io.ReadCloser, error) {
	return r.fs.Open(path)
}

// SetExtensions sets the file extensions to try during resolution
func (r *FileSystemResolver) SetExtensions(extensions []string) {
	r.extensions = extensions
}

// SetIndexFiles sets the index file names to try during resolution
func (r *FileSystemResolver) SetIndexFiles(indexFiles []string) {
	r.indexFiles = indexFiles
}

// SetPriority sets the resolver priority
func (r *FileSystemResolver) SetPriority(priority int) {
	r.priority = priority
}

// fsWrapper wraps a generic fs.FS to implement ModuleFS
type fsWrapper struct {
	fs.FS
}

func (w *fsWrapper) ReadFile(name string) ([]byte, error) {
	if rfs, ok := w.FS.(fs.ReadFileFS); ok {
		return rfs.ReadFile(name)
	}

	// Fallback implementation
	file, err := w.FS.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return io.ReadAll(file)
}

// osFS implements ModuleFS using the OS file system
type osFS struct {
	baseDir string
}

// resolveOSPath joins name onto baseDir, unless name is already an absolute
// OS path (#232) - e.g. a relative import ("./helper.mjs") resolved against
// an entry module whose own path is absolute, which calculateTargetPath's
// relative-import branch (pathpkg.Join(pathpkg.Dir(fromPath), specifier))
// correctly leaves absolute. filepath.Join doesn't special-case an absolute
// later argument - Join(baseDir, "/tmp/helper.mjs") would come out as
// ".../baseDir/tmp/helper.mjs", silently wrong rather than erroring - so an
// already-absolute name must bypass baseDir entirely and be used as-is.
func (osfs *osFS) resolveOSPath(name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(osfs.baseDir, name)
}

func (osfs *osFS) Open(name string) (fs.File, error) {
	return os.Open(osfs.resolveOSPath(name))
}

func (osfs *osFS) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(osfs.resolveOSPath(name))
}

func (osfs *osFS) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(osfs.resolveOSPath(name))
}

func (osfs *osFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(osfs.resolveOSPath(name))
}

// Glob implements fs.GlobFS
func (osfs *osFS) Glob(pattern string) ([]string, error) {
	fullPattern := filepath.Join(osfs.baseDir, pattern)
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, err
	}

	// Remove base directory prefix from results
	result := make([]string, len(matches))
	for i, match := range matches {
		rel, err := filepath.Rel(osfs.baseDir, match)
		if err != nil {
			result[i] = match
		} else {
			result[i] = rel
		}
	}

	return result, nil
}

// Sub implements fs.SubFS
func (osfs *osFS) Sub(dir string) (fs.FS, error) {
	fullDir := filepath.Join(osfs.baseDir, dir)
	return &osFS{baseDir: fullDir}, nil
}
