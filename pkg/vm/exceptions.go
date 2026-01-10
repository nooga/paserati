package vm

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
)

// --- Exception State ---

// Exception state fields for VM (these will be added to the VM struct in vm.go)
type ExceptionState struct {
	currentException Value // Current thrown exception
	unwinding        bool  // True during exception unwinding
}

// --- Exception Handler Operations ---

// findAllExceptionHandlers searches the exception table for all handlers that cover the given PC
// This is needed for finally blocks which may coexist with catch handlers
func (vm *VM) findAllExceptionHandlers(pc int) []*ExceptionHandler {
	// Safety check: ensure we have frames
	if vm.frameCount == 0 {
		return nil
	}

	frame := &vm.frames[vm.frameCount-1]
	if frame.closure == nil {
		// No closure means no exception table
		return nil
	}

	chunk := frame.closure.Fn.Chunk
	var handlers []*ExceptionHandler

	for i := range chunk.ExceptionTable {
		handler := &chunk.ExceptionTable[i]
		if pc >= handler.TryStart && pc < handler.TryEnd {
			handlers = append(handlers, handler)
		}
	}
	return handlers
}

// throwException initiates exception unwinding with the given value
func (vm *VM) throwException(value Value) {
	if debugExceptions {
		fmt.Printf("[DEBUG exceptions.go] throwException called, exception=%s, frameCount=%d, unwinding=%v, crossedNative=%v\n",
			value.ToString(), vm.frameCount, vm.unwinding, vm.unwindingCrossedNative)
	}
	// Avoid double-throwing the same value in a single unwinding sequence
	// EXCEPT when crossing native boundaries (unwindingCrossedNative=true)
	// In that case, this is a legitimate re-throw from native code back into bytecode
	if vm.unwinding && vm.currentException.Is(value) && !vm.unwindingCrossedNative {
		if debugExceptions {
			fmt.Printf("[DEBUG exceptions.go] Duplicate throw of same exception during unwind; ignoring rethrow\n")
		}
		return
	}

	// If we're not already unwinding, this is a fresh throw - reset the flag
	if !vm.unwinding {
		vm.unwindingCrossedNative = false
	}
	// If already unwinding, keep the flag (we're re-throwing after native propagation)

	vm.currentException = value
	vm.unwinding = true
	vm.lastThrownException = value

	// If we're inside a finally block (finallyDepth > 0), a new exception thrown
	// here overrides any pending exception from the original try/catch block.
	// Per ECMAScript spec, the new exception takes precedence.
	if vm.finallyDepth > 0 && vm.pendingAction == ActionThrow {
		vm.pendingAction = ActionNone
		vm.pendingValue = Undefined
	}

	// Start unwinding from current frame
	handlerFound := vm.unwindException()
	if debugExceptions {
		fmt.Printf("[DEBUG exceptions.go] unwindException returned %v, frameCount=%d, unwinding=%v\n", handlerFound, vm.frameCount, vm.unwinding)
	}
	if !handlerFound {
		// No handler found - check if we're in a generator prologue
		// Generator prologues suppress uncaught exception printing
		inGeneratorPrologue := false
		if vm.frameCount > 0 && vm.frames[vm.frameCount-1].isGeneratorPrologue {
			inGeneratorPrologue = true
		}

		if !inGeneratorPrologue {
			// Normal case - print uncaught exception
			if debugExceptions {
				fmt.Printf("[DEBUG exceptions.go] No handler found, calling handleUncaughtException\n")
			}
			vm.handleUncaughtException()
		} else {
			// Generator prologue case - don't print, let caller handle it
			if debugExceptions {
				fmt.Printf("[DEBUG exceptions.go] No handler found but in generator prologue, suppressing print\n")
			}
		}
	}
}

var throwCount = 0

// Helper function to get caller information for debugging
func getCallerInfo() string {
	// Simple debug helper - just return a basic identifier
	return "unknown"
}

// unwindException searches for exception handlers in the current and outer frames
// Returns true if a handler was found, false if exception should terminate execution
func (vm *VM) unwindException() bool {
	if debugExceptions {
		fmt.Printf("[DEBUG unwindException] Starting unwind with frameCount=%d, crossedNative=%v\n",
			vm.frameCount, vm.unwindingCrossedNative)
	}
	for vm.frameCount > 0 {
		frame := &vm.frames[vm.frameCount-1]
		frameName := "unknown"
		if frame.closure != nil && frame.closure.Fn != nil {
			frameName = frame.closure.Fn.Name
		}

		if debugExceptions {
			fmt.Printf("[DEBUG unwindException] Checking frame %d (%s) at IP %d, isDirectCall=%v, isNativeFrame=%v\n",
				vm.frameCount-1, frameName, frame.ip, frame.isDirectCall, frame.isNativeFrame)
		}

		// Look for handlers covering the current IP FIRST
		// Even in direct call frames (generators/async), we want to handle exceptions within the frame
		handlers := vm.findAllExceptionHandlers(frame.ip)

		if debugExceptions {
			fmt.Printf("[DEBUG unwindException] Looking for handlers at IP %d, found %d handlers\n", frame.ip, len(handlers))
			for i, h := range handlers {
				fmt.Printf("[DEBUG unwindException]   Handler %d: TryStart=%d, TryEnd=%d, HandlerPC=%d, IsCatch=%v, IsFinally=%v\n",
					i, h.TryStart, h.TryEnd, h.HandlerPC, h.IsCatch, h.IsFinally)
			}
		}

		for _, handler := range handlers {
			if handler.IsCatch {
				// fmt.Printf("[DEBUG] unwindException: Found catch handler at PC %d\n", handler.HandlerPC)
				// Found a catch handler - execute it
				vm.handleCatchBlock(handler)
				return true
			} else if handler.IsFinally {
				// fmt.Printf("[DEBUG] unwindException: Found finally handler at PC %d\n", handler.HandlerPC)
				// Found a finally handler - execute it and continue unwinding if needed
				vm.handleFinallyBlock(handler)
				// If exception was not cleared (no pending action override), continue unwinding
				if vm.unwinding {
					// fmt.Printf("[DEBUG] unwindException: Still unwinding after finally, continuing\n")
					continue // Continue looking for catch handlers
				} else {
					// fmt.Printf("[DEBUG] unwindException: Finally handled the situation\n")
					return true // Finally handled the situation
				}
			}
		}

		// No handler found in this frame
		// Check if this is a direct call frame (native function boundary)
		// Direct call frames are created by CallFunctionDirectly or executeUserFunctionSafe
		if frame.isDirectCall {
			// Only stop on FIRST PASS (haven't crossed native yet)
			if !vm.unwindingCrossedNative {
				if debugExceptions {
					fmt.Printf("[DEBUG unwindException] Hit direct call boundary at frame %d on FIRST PASS; marking crossed and stopping\n", vm.frameCount-1)
				}
				// Mark that we're crossing into native code
				vm.unwindingCrossedNative = true
				return true // Stop here, let native code handle it
			} else {
				if debugExceptions {
					fmt.Printf("[DEBUG unwindException] Hit direct call boundary at frame %d on RE-THROW PASS; continuing unwinding\n", vm.frameCount-1)
				}
				// On RE-THROW (already crossed native), don't stop - continue unwinding
			}
		}

		// fmt.Printf("[DEBUG] unwindException: No handler in frame %d, unwinding to caller\n", vm.frameCount-1)
		// No handler in current frame and not a direct call boundary, unwind to caller
		vm.frameCount--
		// For register-based VM, just decrement frame count
		// Register cleanup is handled by the frame management
	}

	// fmt.Printf("[DEBUG] unwindException: No handler found in any frame\n")
	// No handler found in any frame
	return false
}

// handleCatchBlock transfers control to a catch block
func (vm *VM) handleCatchBlock(handler *ExceptionHandler) {
	frame := &vm.frames[vm.frameCount-1]
	if debugExceptions {
		fmt.Printf("[DEBUG handleCatchBlock] CatchReg=%d, HandlerPC=%d, exception=%s, crossedNative=%v\n",
			handler.CatchReg, handler.HandlerPC, vm.currentException.ToString(), vm.unwindingCrossedNative)
	}

	// Store exception in catch register if specified
	if handler.CatchReg >= 0 && handler.CatchReg < len(frame.registers) {
		frame.registers[handler.CatchReg] = vm.currentException
		if debugExceptions {
			fmt.Printf("[DEBUG handleCatchBlock] Stored exception in register R%d\n", handler.CatchReg)
		}
	}

	// Jump to catch handler
	frame.ip = handler.HandlerPC
	if debugExceptions {
		fmt.Printf("[DEBUG handleCatchBlock] Jumped to catch handler at PC %d\n", handler.HandlerPC)
	}

	// Clear exception state
	vm.currentException = Null
	vm.unwinding = false
	vm.unwindingCrossedNative = false // NEW: Reset flag
	// Only set handlerFound when we're inside a helper function call
	// This allows the helper's caller to know it needs to jump to the handler
	if vm.helperCallDepth > 0 {
		vm.handlerFound = true
	}
}

// handleFinallyBlock transfers control to a finally block
func (vm *VM) handleFinallyBlock(handler *ExceptionHandler) {
	frame := &vm.frames[vm.frameCount-1]

	// fmt.Printf("[DEBUG] handleFinallyBlock: Entering finally handler at PC %d\n", handler.HandlerPC)

	// Save current exception state if unwinding
	if vm.unwinding {
		// fmt.Printf("[DEBUG] handleFinallyBlock: Saving exception %s as pending action\n", vm.currentException.ToString())
		// Save the exception for later re-throwing
		vm.pendingAction = ActionThrow
		vm.pendingValue = vm.currentException
		// Temporarily clear unwinding so finally block executes normally
		vm.unwinding = false
		vm.currentException = Null
	} else {
		// fmt.Printf("[DEBUG] handleFinallyBlock: Not unwinding, just jumping to finally\n")
	}

	// Jump to finally handler
	frame.ip = handler.HandlerPC
	vm.finallyDepth++

	// fmt.Printf("[DEBUG] handleFinallyBlock: Set IP to %d, finallyDepth=%d\n", handler.HandlerPC, vm.finallyDepth)

	// Note: After the finally block executes, the main VM loop will
	// check for pending actions and execute them appropriately.
}

// handleUncaughtException handles uncaught exceptions by terminating execution
func (vm *VM) handleUncaughtException() {
	// fmt.Printf("[DEBUG] handleUncaughtException called, exception=%s\n", vm.currentException.ToString())
	// For display, use proper string representation
	var displayStr string
	var stackTrace string
	if vm.currentException.IsObject() {
		// For Error objects, try to get a meaningful representation
		if vm.currentException.Type() == TypeObject {
			obj := vm.currentException.AsPlainObject()

			// Check if this looks like an Error object (has name or message properties)
			// First try to get name and message
			nameVal, hasName := obj.GetOwn("name")
			messageVal, hasMessage := obj.GetOwn("message")

			if hasName || hasMessage {
				// If we have name and/or message, format like Error.prototype.toString()
				var name, message string
				if hasName {
					name = nameVal.ToString()
				} else {
					name = "Error" // Default if no name property
				}
				if hasMessage {
					message = messageVal.ToString()
				}

				// Format the error
				if message == "" {
					displayStr = name
				} else {
					displayStr = name + ": " + message
				}

				// Try to get stack trace from Error object
				if stackVal, hasStack := obj.GetOwn("stack"); hasStack {
					stackTrace = stackVal.ToString()
				}
			} else {
				displayStr = vm.currentException.ToString()
			}
		} else {
			displayStr = vm.currentException.ToString()
		}
	} else {
		displayStr = vm.currentException.ToString()
	}

	// Add the uncaught exception as a runtime error (with stack trace if available)
	errorMsg := fmt.Sprintf("Uncaught exception: %s", displayStr)
	if stackTrace != "" {
		errorMsg += "\n" + stackTrace
	}

	// Create runtime error directly without the extra printing from vm.runtimeError
	runtimeErr := &errors.RuntimeError{
		Position: errors.Position{Line: 1, Column: 1, StartPos: 0, EndPos: 0},
		Msg:      errorMsg,
	}
	vm.errors = append(vm.errors, runtimeErr)

	// Keep unwinding = true so the VM knows to terminate
	// vm.unwinding = false // DON'T clear this - we need it to signal termination
	// Set frameCount to 0 to terminate execution
	vm.frameCount = 0
}

// --- OpThrow Implementation ---

// executeOpThrow implements the OpThrow opcode
// This should be called from the main VM run loop in vm.go
func (vm *VM) executeOpThrow(code []byte, ip *int) {
	// Read register containing exception value
	exceptionReg := code[*ip]
	*ip++

	frame := &vm.frames[vm.frameCount-1]
	if int(exceptionReg) >= len(frame.registers) {
		vm.runtimeError("Invalid register index %d for throw operation", exceptionReg)
		return
	}
	exceptionValue := frame.registers[exceptionReg]

	// fmt.Printf("[DEBUG] executeOpThrow: About to throw exception %s from register R%d\n", exceptionValue.ToString(), exceptionReg)

	if debugExceptions {
		fmt.Printf("[DEBUG executeOpThrow] Throwing exception from R%d value=%s (%s)\n", exceptionReg, exceptionValue.Inspect(), exceptionValue.TypeName())
	}
	// Throw the exception
	vm.throwException(exceptionValue)
}

// --- Stack Trace Support (Phase 4b) ---

// StackFrame represents a single frame in a stack trace
type StackFrame struct {
	FunctionName string
	FileName     string
	Line         int
	Column       int
}

// CaptureStackTrace captures the current call stack and returns it as a formatted string
func (vm *VM) CaptureStackTrace() string {
	frames := vm.getStackFrames()
	if len(frames) == 0 {
		return ""
	}

	result := ""
	for i, frame := range frames {
		if i > 0 {
			result += "\n"
		}
		result += fmt.Sprintf("    at %s (%s:%d:%d)", frame.FunctionName, frame.FileName, frame.Line, frame.Column)
	}
	return result
}

// getStackFrames extracts stack frame information from the current VM call stack
func (vm *VM) getStackFrames() []StackFrame {
	var frames []StackFrame

	// Walk through all active frames
	for i := vm.frameCount - 1; i >= 0; i-- {
		frame := &vm.frames[i]

		// Skip native frames - they don't have meaningful source location info
		if frame.isNativeFrame {
			continue
		}

		if frame.closure != nil && frame.closure.Fn != nil {
			fn := frame.closure.Fn

			// Get function name
			funcName := fn.Name
			if funcName == "" {
				funcName = "<anonymous>"
			}

			// Get current line number from chunk's line info
			line := 1
			column := 1
			if fn.Chunk != nil && frame.ip >= 0 && frame.ip < len(fn.Chunk.Lines) {
				line = fn.Chunk.Lines[frame.ip]
			}

			// For now, use a placeholder filename - could be enhanced with source mapping
			fileName := "<script>"
			if funcName != "<script>" && funcName != "<anonymous>" {
				fileName = "<" + funcName + ">"
			}

			frames = append(frames, StackFrame{
				FunctionName: funcName,
				FileName:     fileName,
				Line:         line,
				Column:       column,
			})
		}
	}

	return frames
}
