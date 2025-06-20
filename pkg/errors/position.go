package errors

import "paserati/pkg/source"

// Position represents a specific location in the source code.
// It includes line and column numbers (1-based) for human-readability,
// and byte offsets (0-based) for potential use in tooling (like LSP).
type Position struct {
	Line     int                // 1-based line number
	Column   int                // 1-based column number (rune index within the line)
	StartPos int                // 0-based byte offset of the start of the token/error span
	EndPos   int                // 0-based byte offset of the end of the token/error span (exclusive)
	Source   *source.SourceFile // Reference to the source file
}
