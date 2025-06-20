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

// throwException initiates exception unwinding with the given value
func (vm *VM) throwException(value Value) {
	vm.currentException = value
	vm.unwinding = true
	
	// Start unwinding from current frame
	if !vm.unwindException() {
		// No handler found, terminate with uncaught exception
		vm.handleUncaughtException()
	}
}

// unwindException searches for exception handlers in the current and outer frames
// Returns true if a handler was found, false if exception should terminate execution
func (vm *VM) unwindException() bool {
	for vm.frameCount > 0 {
		frame := &vm.frames[vm.frameCount-1]
		handler := vm.findExceptionHandler(frame.ip)
		
		if handler != nil && handler.IsCatch {
			// Found a catch handler
			vm.handleCatchBlock(handler)
			return true
		}
		
		// No handler in current frame, unwind to caller
		vm.frameCount--
		// For register-based VM, just decrement frame count
		// Register cleanup is handled by the frame management
	}
	
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

// handleUncaughtException handles uncaught exceptions by terminating execution
func (vm *VM) handleUncaughtException() {
	fmt.Printf("Uncaught exception: %s\n", vm.currentException.Inspect())
	vm.unwinding = false
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
	
	// Throw the exception
	vm.throwException(exceptionValue)
}