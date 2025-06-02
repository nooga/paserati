package errors

import (
	"fmt"
	"os"
	"strings"
)

// PaseratiError is the interface implemented by all Paserati errors.
type PaseratiError interface {
	error // Embed the standard error interface
	Pos() Position
	Kind() string // e.g., "Syntax", "Type", "Compile", "Runtime"
	// Message returns the specific error message without position info.
	// This might be useful if the caller wants to format the error differently.
	Message() string
	Unwrap() error // For error wrapping support (errors.Is/As)
}

// --- Concrete Error Types ---

// SyntaxError represents an error during lexing or parsing.
type SyntaxError struct {
	Position
	Msg   string
	Cause error // Underlying cause, if any
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("Syntax Error at %d:%d: %s", e.Line, e.Column, e.Msg)
}
func (e *SyntaxError) Pos() Position   { return e.Position }
func (e *SyntaxError) Kind() string    { return "Syntax" }
func (e *SyntaxError) Message() string { return e.Msg }
func (e *SyntaxError) Unwrap() error   { return e.Cause }
func (e *SyntaxError) CausedBy(cause error) *SyntaxError {
	e.Cause = cause
	return e
}

// TypeError represents an error during static type checking.
type TypeError struct {
	Position
	Msg   string
	Cause error // Underlying cause, if any
}

func (e *TypeError) Error() string {
	return fmt.Sprintf("Type Error at %d:%d: %s", e.Line, e.Column, e.Msg)
}
func (e *TypeError) Pos() Position   { return e.Position }
func (e *TypeError) Kind() string    { return "Type" }
func (e *TypeError) Message() string { return e.Msg }
func (e *TypeError) Unwrap() error   { return e.Cause }
func (e *TypeError) CausedBy(cause error) *TypeError {
	e.Cause = cause
	return e
}

// CompileError represents an error during bytecode compilation.
type CompileError struct {
	Position
	Msg   string
	Cause error // Underlying cause, if any
}

func (e *CompileError) Error() string {
	// Compile errors might sometimes lack precise position,
	// but we include it for consistency.
	// We might refine formatting later based on whether Pos is zero.
	return fmt.Sprintf("Compile Error at %d:%d: %s", e.Line, e.Column, e.Msg)
}
func (e *CompileError) Pos() Position   { return e.Position }
func (e *CompileError) Kind() string    { return "Compile" }
func (e *CompileError) Message() string { return e.Msg }
func (e *CompileError) Unwrap() error   { return e.Cause }
func (e *CompileError) CausedBy(cause error) *CompileError {
	e.Cause = cause
	return e
}

// RuntimeError represents an error during program execution in the VM.
type RuntimeError struct {
	// Position might be less precise for runtime errors, potentially
	// pointing to the start of the operation that failed rather than
	// a specific token. We'll still store it.
	Position
	Msg   string
	Cause error // Underlying cause, if any
}

func (e *RuntimeError) Error() string {
	// Similar to CompileError, we might refine formatting based on Position validity.
	return fmt.Sprintf("Runtime Error at %d:%d: %s", e.Line, e.Column, e.Msg)
}
func (e *RuntimeError) Pos() Position   { return e.Position }
func (e *RuntimeError) Kind() string    { return "Runtime" }
func (e *RuntimeError) Message() string { return e.Msg }
func (e *RuntimeError) Unwrap() error   { return e.Cause }
func (e *RuntimeError) CausedBy(cause error) *RuntimeError {
	e.Cause = cause
	return e
}

// --- Helper for creating errors ---
// (We might add helper functions here later if needed, e.g., NewSyntaxError)

// --- Error Reporting ---

// DisplayErrors prints a list of Paserati errors to stderr in a user-friendly format,
// including the source line and position marker.
func DisplayErrors(source string, errors []PaseratiError) {
	if len(errors) == 0 {
		return
	}

	lines := strings.Split(source, "\n")

	for _, err := range errors {
		pos := err.Pos()
		kind := err.Kind()
		msg := err.Message()

		// Print error location and message
		// Format: <Kind> Error at <Line>:<Column>: <Message>
		fmt.Fprintf(os.Stderr, "%s Error at %d:%d: %s\n", kind, pos.Line, pos.Column, msg)

		// Ensure line numbers are within bounds (1-based index)
		lineIdx := pos.Line - 1
		if lineIdx < 0 || lineIdx >= len(lines) {
			// Print just the error message if line info is invalid
			fmt.Fprintln(os.Stderr) // Add a blank line
			continue
		}

		sourceLine := lines[lineIdx]
		trimmedLine := strings.TrimRight(sourceLine, "\r\n\t ") // Trim trailing whitespace

		// For very long single lines (like -e commands), show a truncated version
		const maxLineLength = 100 // Maximum characters to show
		showSourceLine := true
		markerColumn := pos.Column

		if len(trimmedLine) > maxLineLength {
			// Calculate a reasonable window around the error position
			start := pos.Column - 40 // Show 40 chars before error
			if start < 0 {
				start = 0
			}

			end := start + maxLineLength
			if end > len(trimmedLine) {
				end = len(trimmedLine)
				start = end - maxLineLength
				if start < 0 {
					start = 0
				}
			}

			// If we're showing a truncated version of a very long line (common with -e),
			// and it's a single line of source, skip showing the source altogether
			if len(lines) == 1 && len(trimmedLine) > 200 {
				showSourceLine = false
			} else {
				// Show truncated version with ellipsis
				prefix := ""
				suffix := ""
				if start > 0 {
					prefix = "..."
				}
				if end < len(trimmedLine) {
					suffix = "..."
				}

				truncatedLine := prefix + trimmedLine[start:end] + suffix
				trimmedLine = truncatedLine
				markerColumn = pos.Column - start + len(prefix)
			}
		}

		if showSourceLine {
			// Print the source line
			fmt.Fprintf(os.Stderr, "  %s\n", trimmedLine)

			// Print the marker line (^)
			if markerColumn >= 0 {
				marker := strings.Repeat(" ", markerColumn) + "^"
				fmt.Fprintf(os.Stderr, "  %s\n", marker)
			}
		}

		fmt.Fprintln(os.Stderr) // Add a blank line between errors
	}
}
