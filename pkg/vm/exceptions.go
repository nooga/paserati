package vm

import (
	"fmt"
	"paserati/pkg/errors"
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
		fmt.Printf("[DEBUG exceptions.go] throwException called, exception=%s, frameCount=%d\n", value.ToString(), vm.frameCount)
	}
	// Avoid double-throwing the same value in a single unwinding sequence
	if vm.unwinding && vm.currentException.Is(value) {
		if debugExceptions {
			fmt.Printf("[DEBUG exceptions.go] Duplicate throw of same exception during unwind; ignoring rethrow\n")
		}
		return
	}

	vm.currentException = value
	vm.unwinding = true
	vm.lastThrownException = value

	// Start unwinding from current frame
	handlerFound := vm.unwindException()
	if debugExceptions {
		fmt.Printf("[DEBUG exceptions.go] unwindException returned %v, frameCount=%d, unwinding=%v\n", handlerFound, vm.frameCount, vm.unwinding)
	}
	if !handlerFound {
		// No handler found, terminate with uncaught exception
		if debugExceptions {
			fmt.Printf("[DEBUG exceptions.go] No handler found, calling handleUncaughtException\n")
		}
		vm.handleUncaughtException()
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
		fmt.Printf("[DEBUG unwindException] Starting unwind with frameCount=%d\n", vm.frameCount)
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

		// Check if this is a direct call frame (native function boundary)
		// Direct call frames are created by CallFunctionDirectly (used by native functions like Array.map)
		if frame.isDirectCall {
			if debugExceptions {
				fmt.Printf("[DEBUG unwindException] Hit direct call boundary at frame %d\n", vm.frameCount-1)
			}

			// Check if there are any outer frames that might have exception handlers
			// If so, we should continue unwinding to let those handlers catch the exception
			if vm.frameCount > 1 {
				// Peek at outer frames to see if any have handlers for the current exception
				// We need to check this before deciding to stop unwinding
				if debugExceptions {
					fmt.Printf("[DEBUG unwindException] Checking if outer frames have handlers before stopping\n")
				}

				// Save the original frame count before temporarily modifying it
				originalFrameCount := vm.frameCount
				// Temporarily skip this frame and check outer frames
				vm.frameCount--
				hasOuterHandlers := false

				// Check remaining frames for handlers
				for i := vm.frameCount - 1; i >= 0 && !hasOuterHandlers; i-- {
					outerFrame := &vm.frames[i]
					if !outerFrame.isDirectCall && outerFrame.closure != nil {
						// Check if this frame has exception handlers at the current IP
						handlers := vm.findAllExceptionHandlers(outerFrame.ip)
						if debugExceptions {
							fmt.Printf("[DEBUG unwindException] Checking outer frame %d at IP %d, found %d handlers\n", i, outerFrame.ip, len(handlers))
						}

						// Also check if this frame has ANY exception handlers (not just at current IP)
						// This is important because the IP might be wrong due to call boundaries
						chunk := outerFrame.closure.Fn.Chunk
						totalHandlers := len(chunk.ExceptionTable)
						if debugExceptions {
							fmt.Printf("[DEBUG unwindException] Frame %d has %d total exception handlers in chunk\n", i, totalHandlers)
						}

						// If there are exception handlers at current IP, check them
						if len(handlers) > 0 {
							for _, handler := range handlers {
								if debugExceptions {
									fmt.Printf("[DEBUG unwindException] Handler at frame %d: TryStart=%d, TryEnd=%d, IsCatch=%v\n",
										i, handler.TryStart, handler.TryEnd, handler.IsCatch)
								}
								if handler.IsCatch {
									hasOuterHandlers = true
									if debugExceptions {
										fmt.Printf("[DEBUG unwindException] Found catch handler in outer frame %d\n", i)
									}
									break
								}
							}
						} else if totalHandlers > 0 {
							// No handlers at current IP, but there are handlers in the chunk
							// Check if any of them are catch handlers - this indicates the frame can handle exceptions
							for j := range chunk.ExceptionTable {
								handler := &chunk.ExceptionTable[j]
								if debugExceptions {
									fmt.Printf("[DEBUG unwindException] Total handler %d at frame %d: TryStart=%d, TryEnd=%d, IsCatch=%v\n",
										j, i, handler.TryStart, handler.TryEnd, handler.IsCatch)
								}
								if handler.IsCatch {
									hasOuterHandlers = true
									if debugExceptions {
										fmt.Printf("[DEBUG unwindException] Found catch handler in outer frame %d (not at current IP, but in chunk)\n", i)
									}
									break
								}
							}
						}
					}
				}

				if hasOuterHandlers {
					// Continue unwinding past the direct call boundary
					fmt.Printf("[DEBUG unwindException] Continuing unwind past direct call boundary to reach outer handlers\n")
					vm.escapedDirectCallBoundary = true

					// Restore the original frame count since we're continuing unwinding
					vm.frameCount = originalFrameCount

					// NOTE: Do not forcibly modify outer frame IP here.
					// handleCatchBlock will set the correct handler PC when we find it below.

					// Now properly unwind past the direct call boundary
					// We need to skip the direct call frame and continue unwinding
					vm.frameCount-- // Skip the direct call boundary frame
					continue
				} else {
					// No outer handlers, stop at direct call boundary
					// Instead of restoring frameCount, pop the direct-call frame so the caller
					// can handle the exception without re-encountering this boundary.
					// Note: Reclaim register space for the popped frame.
					dcFrame := &vm.frames[vm.frameCount-1]
					if dcFrame.closure != nil && dcFrame.closure.Fn != nil {
						vm.nextRegSlot -= dcFrame.closure.Fn.RegisterSize
					}
					vm.frameCount--
					if debugExceptions {
						fmt.Printf("[DEBUG unwindException] Popped direct call frame; returning to caller with error (frameCount=%d)\n", vm.frameCount)
					}
					return true // Let the vm.run() return InterpretRuntimeError to the native caller
				}
			} else {
				// No outer frames, stop unwinding
				if debugExceptions {
					fmt.Printf("[DEBUG unwindException] No outer frames, stopping at direct call boundary\n")
				}
				return true
			}
		}

		// Look for handlers covering the current IP
		handlers := vm.findAllExceptionHandlers(frame.ip)

		// fmt.Printf("[DEBUG] unwindException: Found %d handlers for IP %d\n", len(handlers), frame.ip)

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

		// fmt.Printf("[DEBUG] unwindException: No handler in frame %d, unwinding to caller\n", vm.frameCount-1)
		// No handler in current frame, unwind to caller
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
		fmt.Printf("[DEBUG handleCatchBlock] CatchReg=%d, HandlerPC=%d, exception=%s\n",
			handler.CatchReg, handler.HandlerPC, vm.currentException.ToString())
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

			// Check if this looks like an Error object (has name and message properties)
			if nameVal, hasName := obj.GetOwn("name"); hasName {
				if messageVal, hasMessage := obj.GetOwn("message"); hasMessage {
					name := nameVal.ToString()
					message := messageVal.ToString()
					// Format like Error.prototype.toString() would
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
