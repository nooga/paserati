package errors

import "fmt"

// PaseratiError is the interface implemented by all Paserati errors.
type PaseratiError interface {
	error // Embed the standard error interface
	Pos() Position
	Kind() string // e.g., "Syntax", "Type", "Compile", "Runtime"
	// Message returns the specific error message without position info.
	// This might be useful if the caller wants to format the error differently.
	Message() string
}

// --- Concrete Error Types ---

// SyntaxError represents an error during lexing or parsing.
type SyntaxError struct {
	Position
	Msg string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("Syntax Error at %d:%d: %s", e.Line, e.Column, e.Msg)
}
func (e *SyntaxError) Pos() Position   { return e.Position }
func (e *SyntaxError) Kind() string    { return "Syntax" }
func (e *SyntaxError) Message() string { return e.Msg }

// TypeError represents an error during static type checking.
type TypeError struct {
	Position
	Msg string
}

func (e *TypeError) Error() string {
	return fmt.Sprintf("Type Error at %d:%d: %s", e.Line, e.Column, e.Msg)
}
func (e *TypeError) Pos() Position   { return e.Position }
func (e *TypeError) Kind() string    { return "Type" }
func (e *TypeError) Message() string { return e.Msg }

// CompileError represents an error during bytecode compilation.
type CompileError struct {
	Position
	Msg string
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

// RuntimeError represents an error during program execution in the VM.
type RuntimeError struct {
	// Position might be less precise for runtime errors, potentially
	// pointing to the start of the operation that failed rather than
	// a specific token. We'll still store it.
	Position
	Msg string
}

func (e *RuntimeError) Error() string {
	// Similar to CompileError, we might refine formatting based on Position validity.
	return fmt.Sprintf("Runtime Error at %d:%d: %s", e.Line, e.Column, e.Msg)
}
func (e *RuntimeError) Pos() Position   { return e.Position }
func (e *RuntimeError) Kind() string    { return "Runtime" }
func (e *RuntimeError) Message() string { return e.Msg }

// --- Helper for creating errors ---
// (We might add helper functions here later if needed, e.g., NewSyntaxError)
