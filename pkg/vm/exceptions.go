package vm

import "fmt"

// --- Exception State ---

// Exception state fields for VM (these will be added to the VM struct in vm.go)
type ExceptionState struct {
	currentException Value // Current thrown exception
	unwinding        bool  // True during exception unwinding
}

// --- Exception Handler Operations ---

// findExceptionHandler searches the exception table for a handler that covers the given PC
func (vm *VM) findExceptionHandler(pc int) *ExceptionHandler {
	chunk := vm.frames[vm.frameCount-1].closure.Fn.Chunk
	
	for i := range chunk.ExceptionTable {
		handler := &chunk.ExceptionTable[i]
		if pc >= handler.TryStart && pc < handler.TryEnd {
			return handler
		}
	}
	return nil
}

// findAllExceptionHandlers searches the exception table for all handlers that cover the given PC
// This is needed for finally blocks which may coexist with catch handlers
func (vm *VM) findAllExceptionHandlers(pc int) []*ExceptionHandler {
	// Safety check: ensure we have frames
	if vm.frameCount == 0 {
		return nil
	}
	
	chunk := vm.frames[vm.frameCount-1].closure.Fn.Chunk
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
	vm.currentException = value
	vm.unwinding = true
	
	// Start unwinding from current frame
	handlerFound := vm.unwindException()
	if !handlerFound {
		// No handler found, terminate with uncaught exception
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
	// fmt.Printf("[DEBUG] unwindException: Starting unwind with frameCount=%d\n", vm.frameCount)
	for vm.frameCount > 0 {
		frame := &vm.frames[vm.frameCount-1]
		
		// fmt.Printf("[DEBUG] unwindException: Checking frame %d at IP %d\n", vm.frameCount-1, frame.ip)
		
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
	
	// Store exception in catch register if specified
	if handler.CatchReg >= 0 && handler.CatchReg < len(frame.registers) {
		frame.registers[handler.CatchReg] = vm.currentException
	}
	
	// Jump to catch handler  
	frame.ip = handler.HandlerPC
	
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
	
	fmt.Printf("Uncaught exception: %s\n", displayStr)
	
	// Add the uncaught exception as a runtime error
	vm.runtimeError("Uncaught exception: %s", displayStr)
	
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