package source

import (
	"path/filepath"
	"strings"
)

// SourceFile represents a source file with its content and metadata
type SourceFile struct {
	Name     string   // Display name (e.g., "script.ts", "<stdin>", "<eval>")
	Path     string   // Full file path (empty for REPL/eval)
	Content  string   // The source code content
	lines    []string // Cached split lines (lazy initialization)
}

// NewSourceFile creates a new source file
func NewSourceFile(name, path, content string) *SourceFile {
	return &SourceFile{
		Name:    name,
		Path:    path,
		Content: content,
	}
}

// NewEvalSource creates a source file for eval/REPL input
func NewEvalSource(content string) *SourceFile {
	return &SourceFile{
		Name:    "<eval>",
		Path:    "",
		Content: content,
	}
}

// NewReplSource creates a source file for REPL input  
func NewReplSource(content string) *SourceFile {
	return &SourceFile{
		Name:    "<repl>",
		Path:    "",
		Content: content,
	}
}

// NewStdinSource creates a source file for stdin input
func NewStdinSource(content string) *SourceFile {
	return &SourceFile{
		Name:    "<stdin>",
		Path:    "",
		Content: content,
	}
}

// Lines returns the source split into lines (cached)
func (sf *SourceFile) Lines() []string {
	if sf.lines == nil {
		sf.lines = strings.Split(sf.Content, "\n")
	}
	return sf.lines
}

// DisplayPath returns the best path for display (prefers Path, falls back to Name)
func (sf *SourceFile) DisplayPath() string {
	if sf.Path != "" {
		return sf.Path
	}
	return sf.Name
}

// IsFile returns true if this represents an actual file (has a path)
func (sf *SourceFile) IsFile() bool {
	return sf.Path != ""
}

// Helper functions for creating sources from common patterns

// FromFile creates a SourceFile from a file path and content
func FromFile(filePath, content string) *SourceFile {
	name := filepath.Base(filePath)
	return NewSourceFile(name, filePath, content)
}