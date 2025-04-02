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

		// Ensure line numbers are within bounds (1-based index)
		lineIdx := pos.Line - 1
		if lineIdx < 0 || lineIdx >= len(lines) {
			// Print a generic error if line info is invalid
			fmt.Fprintf(os.Stderr, "%s Error: %s\n", kind, msg)
			continue
		}

		sourceLine := lines[lineIdx]
		trimmedLine := strings.TrimRight(sourceLine, "\r\n\t ") // Trim trailing whitespace for cleaner output

		// Print error location and message
		// Format: <Kind> Error at <Line>:<Column>: <Message>
		fmt.Fprintf(os.Stderr, "%s Error at %d:%d: %s\n", kind, pos.Line, pos.Column, msg)

		// Print the source line
		fmt.Fprintf(os.Stderr, "  %s\n", trimmedLine)

		// Print the marker line (^)
		// Adjust column for potentially trimmed leading whitespace? For now, assume column is relative to original line start.
		marker := strings.Repeat(" ", pos.Column) + "^"
		// TODO: Extend marker with '~' for multi-character spans (using StartPos, EndPos)?
		// marker += strings.Repeat("~", pos.EndPos - pos.StartPos -1) // Needs StartPos/EndPos to be reliable
		fmt.Fprintf(os.Stderr, "  %s\n", marker)
		fmt.Fprintln(os.Stderr) // Add a blank line between errors
	}
}
