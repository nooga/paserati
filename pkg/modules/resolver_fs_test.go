package modules

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFileSystemResolverBasic(t *testing.T) {
	// Create a test file system
	testFS := fstest.MapFS{
		"test.ts": &fstest.MapFile{
			Data: []byte(`export function test() { return "test"; }`),
		},
	}
	
	resolver := NewFileSystemResolver(testFS, "")
	
	if resolver.Name() != "FileSystem" {
		t.Errorf("Expected name 'FileSystem', got '%s'", resolver.Name())
	}
	
	if resolver.Priority() != 100 {
		t.Errorf("Expected priority 100, got %d", resolver.Priority())
	}
}

func TestFileSystemResolverCanResolve(t *testing.T) {
	resolver := NewFileSystemResolver(fstest.MapFS{}, "")
	
	tests := []struct {
		specifier  string
		canResolve bool
	}{
		{"./relative.ts", true},
		{"../parent.ts", true},
		{"/absolute.ts", true},
		{"bare-module", false}, // Not handled by file system resolver
		{"@scoped/module", false},
	}
	
	for _, test := range tests {
		result := resolver.CanResolve(test.specifier)
		if result != test.canResolve {
			t.Errorf("CanResolve('%s') = %v, expected %v", test.specifier, result, test.canResolve)
		}
	}
}

func TestFileSystemResolverResolveExact(t *testing.T) {
	// Create a test file system
	testFS := fstest.MapFS{
		"test.ts": &fstest.MapFile{
			Data: []byte(`export function test() { return "test"; }`),
		},
	}
	
	resolver := NewFileSystemResolver(testFS, "")
	
	resolved, err := resolver.Resolve("./test.ts", "")
	if err != nil {
		t.Errorf("Expected successful resolution, got error: %v", err)
		return
	}
	
	if resolved.Specifier != "./test.ts" {
		t.Errorf("Expected specifier './test.ts', got '%s'", resolved.Specifier)
	}
	
	if resolved.ResolvedPath != "test.ts" {
		t.Errorf("Expected resolved path 'test.ts', got '%s'", resolved.ResolvedPath)
	}
	
	if resolved.Resolver != "FileSystem" {
		t.Errorf("Expected resolver 'FileSystem', got '%s'", resolved.Resolver)
	}
	
	// Test reading the source
	sourceBytes, err := io.ReadAll(resolved.Source)
	if err != nil {
		t.Errorf("Expected to read source, got error: %v", err)
		return
	}
	
	expectedContent := `export function test() { return "test"; }`
	if string(sourceBytes) != expectedContent {
		t.Errorf("Expected source content to match, got: %s", string(sourceBytes))
	}
	
	// Clean up
	resolved.Source.Close()
}

func TestFileSystemResolverResolveWithExtension(t *testing.T) {
	testFS := fstest.MapFS{
		"test.ts": &fstest.MapFile{
			Data: []byte(`export const test = true;`),
		},
		"other.js": &fstest.MapFile{
			Data: []byte(`module.exports = { other: true };`),
		},
	}
	
	resolver := NewFileSystemResolver(testFS, "")
	
	// Should resolve without extension
	resolved, err := resolver.Resolve("./test", "")
	if err != nil {
		t.Errorf("Expected successful resolution, got error: %v", err)
		return
	}
	
	if resolved.ResolvedPath != "test.ts" {
		t.Errorf("Expected resolved path 'test.ts', got '%s'", resolved.ResolvedPath)
	}
	
	resolved.Source.Close()
	
	// Should try .js extension
	resolved2, err := resolver.Resolve("./other", "")
	if err != nil {
		t.Errorf("Expected successful resolution, got error: %v", err)
		return
	}
	
	if resolved2.ResolvedPath != "other.js" {
		t.Errorf("Expected resolved path 'other.js', got '%s'", resolved2.ResolvedPath)
	}
	
	resolved2.Source.Close()
}

func TestFileSystemResolverResolveDirectory(t *testing.T) {
	testFS := fstest.MapFS{
		"utils/index.ts": &fstest.MapFile{
			Data: []byte(`export * from './helper';`),
		},
		"lib/index.js": &fstest.MapFile{
			Data: []byte(`module.exports = require('./main');`),
		},
	}
	
	resolver := NewFileSystemResolver(testFS, "")
	
	// Should resolve directory to index.ts
	resolved, err := resolver.Resolve("./utils", "")
	if err != nil {
		t.Errorf("Expected successful resolution, got error: %v", err)
		return
	}
	
	if resolved.ResolvedPath != "utils/index.ts" {
		t.Errorf("Expected resolved path 'utils/index.ts', got '%s'", resolved.ResolvedPath)
	}
	
	resolved.Source.Close()
	
	// Should resolve directory to index.js
	resolved2, err := resolver.Resolve("./lib", "")
	if err != nil {
		t.Errorf("Expected successful resolution, got error: %v", err)
		return
	}
	
	if resolved2.ResolvedPath != "lib/index.js" {
		t.Errorf("Expected resolved path 'lib/index.js', got '%s'", resolved2.ResolvedPath)
	}
	
	resolved2.Source.Close()
}

func TestFileSystemResolverRelativeResolution(t *testing.T) {
	testFS := fstest.MapFS{
		"src/utils/helper.ts": &fstest.MapFile{
			Data: []byte(`export function help() {}`),
		},
		"src/main.ts": &fstest.MapFile{
			Data: []byte(`import { help } from './utils/helper';`),
		},
	}
	
	resolver := NewFileSystemResolver(testFS, "")
	
	// Resolve relative import from src/main.ts
	resolved, err := resolver.Resolve("./utils/helper", "src/main.ts")
	if err != nil {
		t.Errorf("Expected successful resolution, got error: %v", err)
		return
	}
	
	if resolved.ResolvedPath != "src/utils/helper.ts" {
		t.Errorf("Expected resolved path 'src/utils/helper.ts', got '%s'", resolved.ResolvedPath)
	}
	
	resolved.Source.Close()
}

func TestFileSystemResolverParentDirectory(t *testing.T) {
	testFS := fstest.MapFS{
		"shared/types.ts": &fstest.MapFile{
			Data: []byte(`export interface User { name: string; }`),
		},
		"src/models/user.ts": &fstest.MapFile{
			Data: []byte(`import { User } from '../../shared/types';`),
		},
	}
	
	resolver := NewFileSystemResolver(testFS, "")
	
	// Resolve parent directory import
	resolved, err := resolver.Resolve("../../shared/types", "src/models/user.ts")
	if err != nil {
		t.Errorf("Expected successful resolution, got error: %v", err)
		return
	}
	
	if resolved.ResolvedPath != "shared/types.ts" {
		t.Errorf("Expected resolved path 'shared/types.ts', got '%s'", resolved.ResolvedPath)
	}
	
	resolved.Source.Close()
}

func TestFileSystemResolverFailureCase(t *testing.T) {
	testFS := fstest.MapFS{
		"existing.ts": &fstest.MapFile{
			Data: []byte(`export const exists = true;`),
		},
	}
	
	resolver := NewFileSystemResolver(testFS, "")
	
	// Try to resolve non-existent module
	_, err := resolver.Resolve("./nonexistent.ts", "")
	if err == nil {
		t.Error("Expected error when resolving non-existent module")
	}
	
	if !strings.Contains(err.Error(), "module not found") {
		t.Errorf("Expected 'module not found' error, got: %v", err)
	}
}

func TestFileSystemResolverRelativeWithoutFromPath(t *testing.T) {
	resolver := NewFileSystemResolver(fstest.MapFS{}, "")
	
	// Try to resolve relative import without fromPath (./ should work, ../ should fail)
	_, err := resolver.Resolve("../relative.ts", "")
	if err == nil {
		t.Error("Expected error when resolving parent directory import without fromPath")
	}
	
	if !strings.Contains(err.Error(), "relative import") || !strings.Contains(err.Error(), "requires fromPath") {
		t.Errorf("Expected relative import error, got: %v", err)
	}
}

func TestFileSystemResolverCustomExtensions(t *testing.T) {
	testFS := fstest.MapFS{
		"test.custom": &fstest.MapFile{
			Data: []byte(`export const custom = true;`),
		},
	}
	
	resolver := NewFileSystemResolver(testFS, "")
	resolver.SetExtensions([]string{".custom", ".ts"})
	
	// Should resolve with custom extension
	resolved, err := resolver.Resolve("./test", "")
	if err != nil {
		t.Errorf("Expected successful resolution, got error: %v", err)
		return
	}
	
	if resolved.ResolvedPath != "test.custom" {
		t.Errorf("Expected resolved path 'test.custom', got '%s'", resolved.ResolvedPath)
	}
	
	resolved.Source.Close()
}

func TestFileSystemResolverCustomIndexFiles(t *testing.T) {
	testFS := fstest.MapFS{
		"lib/main.ts": &fstest.MapFile{
			Data: []byte(`export const lib = true;`),
		},
	}
	
	resolver := NewFileSystemResolver(testFS, "")
	resolver.SetIndexFiles([]string{"main.ts", "index.ts"})
	
	// Should resolve directory to main.ts
	resolved, err := resolver.Resolve("./lib", "")
	if err != nil {
		t.Errorf("Expected successful resolution, got error: %v", err)
		return
	}
	
	if resolved.ResolvedPath != "lib/main.ts" {
		t.Errorf("Expected resolved path 'lib/main.ts', got '%s'", resolved.ResolvedPath)
	}
	
	resolved.Source.Close()
}

func TestFileSystemResolverSetPriority(t *testing.T) {
	resolver := NewFileSystemResolver(fstest.MapFS{}, "")
	
	originalPriority := resolver.Priority()
	if originalPriority != 100 {
		t.Errorf("Expected default priority 100, got %d", originalPriority)
	}
	
	resolver.SetPriority(75)
	
	newPriority := resolver.Priority()
	if newPriority != 75 {
		t.Errorf("Expected priority 75 after setting, got %d", newPriority)
	}
}

func TestOSFileSystemResolver(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "paserati_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create test files
	testContent := `export function test() { return "from file"; }`
	testFile := filepath.Join(tempDir, "test.ts")
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	
	// Create nested directory and index file
	utilsDir := filepath.Join(tempDir, "utils")
	err = os.MkdirAll(utilsDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create utils dir: %v", err)
	}
	
	indexContent := `export * from './helper';`
	indexFile := filepath.Join(utilsDir, "index.ts")
	err = os.WriteFile(indexFile, []byte(indexContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write index file: %v", err)
	}
	
	// Test OS file system resolver
	resolver := NewOSFileSystemResolver(tempDir)
	
	if resolver.Name() != "OSFileSystem" {
		t.Errorf("Expected name 'OSFileSystem', got '%s'", resolver.Name())
	}
	
	// Test resolving exact file
	resolved, err := resolver.Resolve("./test.ts", "")
	if err != nil {
		t.Errorf("Expected successful resolution, got error: %v", err)
		return
	}
	
	if resolved.ResolvedPath != "test.ts" {
		t.Errorf("Expected resolved path 'test.ts', got '%s'", resolved.ResolvedPath)
	}
	
	// Read and verify content
	sourceBytes, err := io.ReadAll(resolved.Source)
	if err != nil {
		t.Errorf("Expected to read source, got error: %v", err)
		return
	}
	
	if string(sourceBytes) != testContent {
		t.Errorf("Expected source content to match file content")
	}
	
	resolved.Source.Close()
	
	// Test resolving without extension
	resolved2, err := resolver.Resolve("./test", "")
	if err != nil {
		t.Errorf("Expected successful resolution without extension, got error: %v", err)
		return
	}
	
	if resolved2.ResolvedPath != "test.ts" {
		t.Errorf("Expected resolved path 'test.ts' for extensionless, got '%s'", resolved2.ResolvedPath)
	}
	
	resolved2.Source.Close()
	
	// Test resolving directory to index
	resolved3, err := resolver.Resolve("./utils", "")
	if err != nil {
		t.Errorf("Expected successful directory resolution, got error: %v", err)
		return
	}
	
	if resolved3.ResolvedPath != filepath.Join("utils", "index.ts") {
		t.Errorf("Expected resolved path 'utils/index.ts', got '%s'", resolved3.ResolvedPath)
	}
	
	resolved3.Source.Close()
}