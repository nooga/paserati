package errors

import (
	"fmt"
	"os"
	"strings"
)

// ANSI color codes for enhanced error formatting
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorCyan   = "\033[36m"
	ColorGray   = "\033[90m"
	ColorBold   = "\033[1m"
)

// Error codes for different types of errors (TypeScript-style)
const (
	// Syntax Error Codes (PS1xxx)
	PS1001 = "PS1001" // Unexpected token
	PS1002 = "PS1002" // Missing token
	PS1003 = "PS1003" // Invalid syntax
	
	// Type Error Codes (PS2xxx)
	PS2001 = "PS2001" // Type assignment error
	PS2002 = "PS2002" // Property does not exist
	PS2003 = "PS2003" // Function argument type mismatch
	PS2004 = "PS2004" // Generic constraint violation
	PS2005 = "PS2005" // Type not assignable
	
	// Compile Error Codes (PS3xxx)
	PS3001 = "PS3001" // Compilation failed
	PS3002 = "PS3002" // Bytecode generation error
	
	// Runtime Error Codes (PS4xxx)
	PS4001 = "PS4001" // Runtime exception
	PS4002 = "PS4002" // Reference error
)

// PaseratiError is the interface implemented by all Paserati errors.
type PaseratiError interface {
	error // Embed the standard error interface
	Pos() Position
	Kind() string // e.g., "Syntax", "Type", "Compile", "Runtime"
	Code() string // Error code (e.g., "PS2001")
	// Message returns the specific error message without position info.
	// This might be useful if the caller wants to format the error differently.
	Message() string
	Unwrap() error // For error wrapping support (errors.Is/As)
}

// --- Concrete Error Types ---

// SyntaxError represents an error during lexing or parsing.
type SyntaxError struct {
	Position
	Msg      string
	ErrorCode string // Error code (e.g., PS1001)
	Cause    error  // Underlying cause, if any
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("Syntax Error at %d:%d: %s", e.Line, e.Column, e.Msg)
}
func (e *SyntaxError) Pos() Position   { return e.Position }
func (e *SyntaxError) Kind() string    { return "Syntax" }
func (e *SyntaxError) Code() string    { 
	if e.ErrorCode != "" { 
		return e.ErrorCode 
	}
	return PS1003 // Default syntax error code
}
func (e *SyntaxError) Message() string { return e.Msg }
func (e *SyntaxError) Unwrap() error   { return e.Cause }
func (e *SyntaxError) CausedBy(cause error) *SyntaxError {
	e.Cause = cause
	return e
}
func (e *SyntaxError) WithCode(code string) *SyntaxError {
	e.ErrorCode = code
	return e
}

// TypeError represents an error during static type checking.
type TypeError struct {
	Position
	Msg       string
	ErrorCode string // Error code (e.g., PS2001)
	Cause     error  // Underlying cause, if any
}

func (e *TypeError) Error() string {
	return fmt.Sprintf("Type Error at %d:%d: %s", e.Line, e.Column, e.Msg)
}
func (e *TypeError) Pos() Position   { return e.Position }
func (e *TypeError) Kind() string    { return "Type" }
func (e *TypeError) Code() string    { 
	if e.ErrorCode != "" { 
		return e.ErrorCode 
	}
	return PS2001 // Default type error code
}
func (e *TypeError) Message() string { return e.Msg }
func (e *TypeError) Unwrap() error   { return e.Cause }
func (e *TypeError) CausedBy(cause error) *TypeError {
	e.Cause = cause
	return e
}
func (e *TypeError) WithCode(code string) *TypeError {
	e.ErrorCode = code
	return e
}

// CompileError represents an error during bytecode compilation.
type CompileError struct {
	Position
	Msg       string
	ErrorCode string // Error code (e.g., PS3001)
	Cause     error  // Underlying cause, if any
}

func (e *CompileError) Error() string {
	return fmt.Sprintf("Compile Error at %d:%d: %s", e.Line, e.Column, e.Msg)
}
func (e *CompileError) Pos() Position   { return e.Position }
func (e *CompileError) Kind() string    { return "Compile" }
func (e *CompileError) Code() string    { 
	if e.ErrorCode != "" { 
		return e.ErrorCode 
	}
	return PS3001 // Default compile error code
}
func (e *CompileError) Message() string { return e.Msg }
func (e *CompileError) Unwrap() error   { return e.Cause }
func (e *CompileError) CausedBy(cause error) *CompileError {
	e.Cause = cause
	return e
}
func (e *CompileError) WithCode(code string) *CompileError {
	e.ErrorCode = code
	return e
}

// RuntimeError represents an error during program execution in the VM.
type RuntimeError struct {
	Position
	Msg       string
	ErrorCode string // Error code (e.g., PS4001)
	Cause     error  // Underlying cause, if any
}

func (e *RuntimeError) Error() string {
	return fmt.Sprintf("Runtime Error at %d:%d: %s", e.Line, e.Column, e.Msg)
}
func (e *RuntimeError) Pos() Position   { return e.Position }
func (e *RuntimeError) Kind() string    { return "Runtime" }
func (e *RuntimeError) Code() string    { 
	if e.ErrorCode != "" { 
		return e.ErrorCode 
	}
	return PS4001 // Default runtime error code
}
func (e *RuntimeError) Message() string { return e.Msg }
func (e *RuntimeError) Unwrap() error   { return e.Cause }
func (e *RuntimeError) CausedBy(cause error) *RuntimeError {
	e.Cause = cause
	return e
}
func (e *RuntimeError) WithCode(code string) *RuntimeError {
	e.ErrorCode = code
	return e
}

// --- Helper functions for creating errors ---

// NewSyntaxError creates a new syntax error with optional error code
func NewSyntaxError(pos Position, msg string, code ...string) *SyntaxError {
	err := &SyntaxError{
		Position: pos,
		Msg:      msg,
	}
	if len(code) > 0 {
		err.ErrorCode = code[0]
	}
	return err
}

// NewTypeError creates a new type error with optional error code
func NewTypeError(pos Position, msg string, code ...string) *TypeError {
	err := &TypeError{
		Position: pos,
		Msg:      msg,
	}
	if len(code) > 0 {
		err.ErrorCode = code[0]
	}
	return err
}

// NewCompileError creates a new compile error with optional error code
func NewCompileError(pos Position, msg string, code ...string) *CompileError {
	err := &CompileError{
		Position: pos,
		Msg:      msg,
	}
	if len(code) > 0 {
		err.ErrorCode = code[0]
	}
	return err
}

// NewRuntimeError creates a new runtime error with optional error code
func NewRuntimeError(pos Position, msg string, code ...string) *RuntimeError {
	err := &RuntimeError{
		Position: pos,
		Msg:      msg,
	}
	if len(code) > 0 {
		err.ErrorCode = code[0]
	}
	return err
}

// Specific helper functions for common error types

// NewTypeAssignmentError creates a type assignment error (PS2001)
func NewTypeAssignmentError(pos Position, fromType, toType string) *TypeError {
	msg := fmt.Sprintf("Type '%s' is not assignable to type '%s'", fromType, toType)
	return NewTypeError(pos, msg, PS2001)
}

// NewGenericConstraintError creates a generic constraint violation error (PS2004)
func NewGenericConstraintError(pos Position, argType, constraintType, paramName string) *TypeError {
	msg := fmt.Sprintf("Type '%s' does not satisfy constraint '%s' for type parameter '%s'", 
		argType, constraintType, paramName)
	return NewTypeError(pos, msg, PS2004)
}

// NewPropertyNotExistError creates a property does not exist error (PS2002)
func NewPropertyNotExistError(pos Position, property, objectType string) *TypeError {
	msg := fmt.Sprintf("Property '%s' does not exist on type '%s'", property, objectType)
	return NewTypeError(pos, msg, PS2002)
}

// NewArgumentTypeError creates a function argument type mismatch error (PS2003)
func NewArgumentTypeError(pos Position, argNum int, expectedType, actualType string) *TypeError {
	msg := fmt.Sprintf("Argument %d: cannot assign type '%s' to parameter of type '%s'", 
		argNum, actualType, expectedType)
	return NewTypeError(pos, msg, PS2003)
}

// NewUnexpectedTokenError creates an unexpected token error (PS1001)
func NewUnexpectedTokenError(pos Position, token string) *SyntaxError {
	msg := fmt.Sprintf("Unexpected token '%s'", token)
	return NewSyntaxError(pos, msg, PS1001)
}

// NewMissingTokenError creates a missing token error (PS1002)
func NewMissingTokenError(pos Position, expected string) *SyntaxError {
	msg := fmt.Sprintf("Expected '%s'", expected)
	return NewSyntaxError(pos, msg, PS1002)
}

// --- Error Reporting ---

// isColorTerminal checks if we should use colors (basic detection)
func isColorTerminal() bool {
	// Check common terminal color support environment variables
	term := os.Getenv("TERM")
	colorTerm := os.Getenv("COLORTERM")
	return term != "dumb" && (colorTerm != "" || 
		strings.Contains(term, "color") || 
		strings.Contains(term, "xterm") ||
		strings.Contains(term, "screen"))
}

// colorize applies color to text if terminal supports it
func colorize(color, text string) string {
	if isColorTerminal() {
		return color + text + ColorReset
	}
	return text
}

// DisplayErrors prints a list of Paserati errors in TypeScript-style format with enhanced visual presentation
func DisplayErrors(errors []PaseratiError, fallbackSource ...string) {
	if len(errors) == 0 {
		return
	}
	
	// Use fallback source if no source files are available
	var fallbackLines []string
	if len(fallbackSource) > 0 && fallbackSource[0] != "" {
		fallbackLines = strings.Split(fallbackSource[0], "\n")
	}

	for _, err := range errors {
		pos := err.Pos()
		code := err.Code()
		msg := err.Message()

		// TypeScript-style header: PS2345 [ERROR]: Message
		errorHeader := fmt.Sprintf("%s %s: %s", 
			colorize(ColorBlue, code), 
			colorize(ColorBold+ColorRed, "[ERROR]"), 
			msg)
		fmt.Fprintf(os.Stderr, "%s\n", errorHeader)

		// Get source lines from either source file or fallback
		var lines []string
		if pos.Source != nil {
			lines = pos.Source.Lines()
		} else if fallbackLines != nil {
			lines = fallbackLines
		} else {
			// No source available at all
			locationInfo := colorize(ColorGray, fmt.Sprintf("    at line %d, column %d", pos.Line, pos.Column))
			fmt.Fprintf(os.Stderr, "%s\n", locationInfo)
			fmt.Fprintln(os.Stderr) // Add a blank line
			continue
		}
		
		// Ensure line numbers are within bounds (1-based index)
		lineIdx := pos.Line - 1
		if lineIdx < 0 || lineIdx >= len(lines) {
			fmt.Fprintln(os.Stderr) // Add a blank line
			continue
		}

		sourceLine := lines[lineIdx]
		trimmedLine := strings.TrimRight(sourceLine, "\r\n\t ") // Trim trailing whitespace

		// Handle long lines with intelligent truncation
		const maxLineLength = 120 // Increased for better readability
		showSourceLine := true
		markerColumn := pos.Column
		markerLength := calculateErrorLength(pos) // Default to 1 char

		if len(trimmedLine) > maxLineLength {
			start := pos.Column - 60 // Show more context before error
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

			// Skip source display for extremely long single lines (like -e commands)
			if len(lines) == 1 && len(trimmedLine) > 300 {
				showSourceLine = false
			} else {
				// Show truncated version with clear indicators
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
			// Show source line with line number and indentation
			lineNumber := fmt.Sprintf("%d", pos.Line)
			linePrefix := colorize(ColorGray, fmt.Sprintf("%s: ", lineNumber))
			
			fmt.Fprintf(os.Stderr, "  %s%s\n", linePrefix, trimmedLine)

			// Create enhanced marker line with squiggly underline
			if markerColumn >= 0 && markerColumn <= len(trimmedLine) {
				// Calculate spaces to account for line number prefix
				prefixSpaces := len(lineNumber) + 2 + 2 // "123: " + "  "
				
				// Fix off-by-one error: markerColumn is 1-based, convert to 0-based for spacing
				adjustedColumn := markerColumn - 1
				if adjustedColumn < 0 {
					adjustedColumn = 0
				}
				
				marker := strings.Repeat(" ", prefixSpaces + adjustedColumn)
				
				// Use squiggly underline for better visibility
				underline := strings.Repeat("~", markerLength)
				if markerLength == 1 {
					underline = "^" // Single character errors use caret
				}
				
				markerLine := marker + colorize(ColorRed, underline)
				fmt.Fprintf(os.Stderr, "%s\n", markerLine)
			}
		}

		// Add file location info (TypeScript-style with clickable path)
		var locationInfo string
		if pos.Source != nil && pos.Source.IsFile() {
			locationInfo = colorize(ColorGray, fmt.Sprintf("    at %s:%d:%d", pos.Source.DisplayPath(), pos.Line, pos.Column))
		} else if pos.Source != nil {
			locationInfo = colorize(ColorGray, fmt.Sprintf("    at %s:%d:%d", pos.Source.Name, pos.Line, pos.Column))
		} else {
			locationInfo = colorize(ColorGray, fmt.Sprintf("    at line %d, column %d", pos.Line, pos.Column))
		}
		fmt.Fprintf(os.Stderr, "%s\n", locationInfo)

		fmt.Fprintln(os.Stderr) // Add a blank line between errors
	}
}

// calculateErrorLength determines how many characters to underline
// This is a simple version - could be enhanced to use actual token length
func calculateErrorLength(pos Position) int {
	// Use EndPos if available for more precise underlining
	if pos.EndPos > pos.StartPos {
		return pos.EndPos - pos.StartPos
	}
	return 1 // Default to single character
}
