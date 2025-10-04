package vm

import "fmt"

// VMInitCallback is a function that initializes VM-specific functionality
// It receives the VM instance and can set up prototypes, global objects, etc.
// type VMInitCallback func(vm *VM) error

// // Global registry of initialization callbacks
// var (
// 	globalInitCallbacks []VMInitCallback
// )

// // RegisterGlobalInitCallback registers a callback that will be called
// // for every new VM instance during initialization
// func RegisterGlobalInitCallback(callback VMInitCallback) {
// 	globalInitCallbacks = append(globalInitCallbacks, callback)
// }

// // initializeVM runs all registered initialization callbacks
// func (vm *VM) initializeVM() error {
// 	// Run global callbacks first (if any)
// 	for _, callback := range globalInitCallbacks {
// 		if err := callback(vm); err != nil {
// 			return err
// 		}
// 	}

// 	// Run instance-specific callbacks
// 	for _, callback := range vm.initCallbacks {
// 		if err := callback(vm); err != nil {
// 			return err
// 		}
// 	}

// 	return nil
// }

// // AddStandardCallbacks adds a set of standard callbacks to this VM instance
// // This allows external packages to provide standard initialization without circular dependencies
// func (vm *VM) AddStandardCallbacks(callbacks []VMInitCallback) {
// 	vm.initCallbacks = append(vm.initCallbacks, callbacks...)
// }

// // InitializeWithCallbacks runs the initialization callbacks that were added to this VM
// // This is separate from the constructor to allow adding callbacks after VM creation
// func (vm *VM) InitializeWithCallbacks() error {
// 	return vm.initializeVM()
// }

// vmCaller implements the VMCaller interface for async native functions
type vmCaller struct {
	vm           *VM
	yieldCh      chan *BytecodeCall
	currentFrame *CallFrame
}

func (vc *vmCaller) CallBytecode(fn Value, thisValue Value, args []Value) Value {
	// Create a channel to receive the result
	resultCh := make(chan Value, 1)

	// Create the bytecode call request
	call := &BytecodeCall{
		Function:  fn,
		ThisValue: thisValue,
		Args:      args,
		ResultCh:  resultCh,
	}

	// Send the call request to the VM
	vc.yieldCh <- call

	// Wait for the result
	result := <-resultCh
	return result
}

// executeAsyncNativeFunction executes an async native function that can call bytecode
func (vm *VM) executeAsyncNativeFunction(asyncFn *AsyncNativeFunctionObject, args []Value, destReg byte, callerRegisters []Value) (Value, error) {
	// Create channels for communication
	yieldCh := make(chan *BytecodeCall, 1)
	completeCh := make(chan Value, 1)

	// Create the VM caller interface
	caller := &vmCaller{
		vm:      vm,
		yieldCh: yieldCh,
	}

	// Run the async native function in a goroutine
	go func() {
		result := asyncFn.AsyncFn(caller, args)
		completeCh <- result
	}()

	// Process bytecode calls and wait for completion
	for {
		select {
		case call := <-yieldCh:
			// Execute the bytecode call
			result, err := vm.executeUserFunctionReentrant(call.Function, call.ThisValue, call.Args)
			if err != nil {
				call.ResultCh <- Undefined
			} else {
				call.ResultCh <- result
			}

		case result := <-completeCh:
			// Async function completed
			if int(destReg) < len(callerRegisters) {
				callerRegisters[destReg] = result
			}
			return result, nil
		}
	}
}

// executeUserFunctionReentrant executes a user-defined function from within a builtin
// This creates a minimal execution context similar to how modern JS engines handle builtin->JS calls
func (vm *VM) executeUserFunctionReentrant(fn Value, thisValue Value, args []Value) (Value, error) {

	// Check if we have space for another frame
	if vm.frameCount >= MaxFrames {
		return Undefined, fmt.Errorf("stack overflow during re-entrant call")
	}

	// Use the existing prepareCall infrastructure
	// Create dummy caller registers and IP for the context
	dummyCallerRegisters := make([]Value, 1) // Just need space for result
	dummyCallerIP := 0
	dummyDestReg := byte(0)

	// Use prepareDirectCall so the created frame is marked as a direct-call boundary
	shouldSwitch, err := vm.prepareDirectCall(fn, thisValue, args, dummyDestReg, dummyCallerRegisters, dummyCallerIP)
	if err != nil {
		return Undefined, fmt.Errorf("failed to prepare re-entrant call: %v", err)
	}

	if !shouldSwitch {
		// Native function was executed directly, return the result
		return dummyCallerRegisters[dummyDestReg], nil
	}

	// We have a new frame for bytecode execution, run the interpreter
	// The new frame is set up, now run the VM until it returns
	// Since prepareCall set up the frame, we can just call run()
	status, _ := vm.run()

	if status == InterpretRuntimeError {
		// If VM is unwinding and has a currentException, surface it as an ExceptionError
		if vm.unwinding && vm.currentException != Null {
			return Undefined, exceptionError{exception: vm.currentException}
		}
		return Undefined, fmt.Errorf("runtime error during re-entrant execution")
	}

	// The function should have returned and placed its result in dummyCallerRegisters[0]
	return dummyCallerRegisters[dummyDestReg], nil
}

// Deprecated: ExecuteUserFunctionForBuiltin has been removed. Use vm.Call instead.

// RegisterInitCallback registers a callback for this specific VM instance
// func (vm *VM) RegisterInitCallback(callback VMInitCallback) {
// 	vm.initCallbacks = append(vm.initCallbacks, callback)
// }

// initializePrototypes sets up the built-in prototype objects
func (vm *VM) initializePrototypes() {
	// Create the root Object.prototype (with null prototype)
	vm.ObjectPrototype = NewObject(Null)

	// Function.prototype inherits from Object.prototype
	vm.FunctionPrototype = NewObject(vm.ObjectPrototype)

	// Array.prototype inherits from Object.prototype
	vm.ArrayPrototype = NewObject(vm.ObjectPrototype)

	// String.prototype inherits from Object.prototype
	vm.StringPrototype = NewObject(vm.ObjectPrototype)

	// Number.prototype inherits from Object.prototype
	vm.NumberPrototype = NewObject(vm.ObjectPrototype)

	// Boolean.prototype inherits from Object.prototype
	vm.BooleanPrototype = NewObject(vm.ObjectPrototype)

	// Error.prototype inherits from Object.prototype
	vm.ErrorPrototype = NewObject(vm.ObjectPrototype)

	// TypeError.prototype inherits from Error.prototype
	vm.TypeErrorPrototype = NewObject(vm.ErrorPrototype)

	// ReferenceError.prototype inherits from Error.prototype
	vm.ReferenceErrorPrototype = NewObject(vm.ErrorPrototype)

	// Symbol.prototype inherits from Object.prototype
	vm.SymbolPrototype = NewObject(vm.ObjectPrototype)
}

// Deprecated: CallFunctionFromBuiltin has been removed. Builtins should use vm.Call.

// CallFunctionDirectly executes a user-defined function directly without re-entrant execution
// This is specifically designed for Function.prototype.call to avoid infinite recursion
func (vm *VM) CallFunctionDirectly(fn Value, thisValue Value, args []Value) (Value, error) {
	// fmt.Printf("[DEBUG CallFunctionDirectly] Called with fn type=%d, args=%d\n", fn.Type(), len(args))
	// Only handle user-defined functions and closures
	if !fn.IsFunction() && !fn.IsClosure() {
		return Undefined, fmt.Errorf("CallFunctionDirectly: not a user-defined function")
	}

	// Check if we have space for another frame
	if vm.frameCount >= MaxFrames {
		return Undefined, fmt.Errorf("stack overflow during direct function call")
	}

	// Get function arity and adjust arguments accordingly
	var expectedArity int
	if fn.IsFunction() {
		fnObj := fn.AsFunction()
		expectedArity = fnObj.Arity
	} else if fn.IsClosure() {
		closureObj := fn.AsClosure()
		expectedArity = closureObj.Fn.Arity
	}

	// Truncate arguments to match expected arity (JavaScript allows extra arguments to be ignored)
	adjustedArgs := args
	if len(args) > expectedArity {
		adjustedArgs = args[:expectedArity]
	}

	// Create registers for the call result
	resultRegisters := make([]Value, 1)
	dummyCallerIP := 0
	destReg := byte(0)

	// Use prepareDirectCall to set up the function call with isDirectCall flag
	shouldSwitch, err := vm.prepareDirectCall(fn, thisValue, adjustedArgs, destReg, resultRegisters, dummyCallerIP)
	if err != nil {
		return Undefined, fmt.Errorf("failed to prepare direct call: %v", err)
	}

	if !shouldSwitch {
		// Native function was executed directly, return the result
		return resultRegisters[destReg], nil
	}

	// We have a new frame for bytecode execution with isDirectCall = true
	// Execute the VM run loop - it will return immediately when the frame returns
	if debugCalls {
		fmt.Printf("[DEBUG CallFunctionDirectly] About to execute bytecode, frameCount=%d\n", vm.frameCount)
	}
	initialFrameCount := vm.frameCount
	status, result := vm.run()
	currentFrameCount := vm.frameCount
	if debugCalls {
		fmt.Printf("[DEBUG CallFunctionDirectly] Bytecode execution finished, status=%d, result=%s, frameCount=%d->%d\n", status, result.Inspect(), initialFrameCount, currentFrameCount)
	}

	if status == InterpretRuntimeError {
		// If VM is unwinding and has a currentException, surface it as an ExceptionError
		if vm.unwinding && vm.currentException != Null {
			return Undefined, exceptionError{exception: vm.currentException}
		}
		return Undefined, fmt.Errorf("runtime error during direct function execution")
	}

	// Check if the frame count dropped to 0 - this indicates the entire script execution
	// was completed due to an exception being caught by an outer handler
	if currentFrameCount == 0 {
		if debugCalls {
			fmt.Printf("[DEBUG CallFunctionDirectly] Frame count dropped to 0 (from %d) - script execution completed\n", initialFrameCount)
		}
		// The script execution has completed. This means we're no longer in a callback context
		// but the entire program has terminated. Signal this to the caller.
		// IMPORTANT: Do not return the script's final result value here, as that can
		// corrupt native method return paths. Return undefined with the special error signal.
		return Undefined, fmt.Errorf("SCRIPT_COMPLETED_WITH_RESULT: %s", result.Inspect())
	}

	return result, nil
}

// IsUnwinding returns true if the VM is currently in an exception unwinding state
func (vm *VM) IsUnwinding() bool {
	return vm.unwinding
}

// GetFrameCount returns the current frame count for debugging
func (vm *VM) GetFrameCount() int {
	return vm.frameCount
}

// Call is a unified function calling interface that handles all function types properly
// This replaces the complex web of CallFunctionDirectly, CallUserFunction, etc.
func (vm *VM) Call(fn Value, thisValue Value, args []Value) (Value, error) {
	switch fn.Type() {
	case TypeNativeFunction:
		// For native functions, call directly with proper 'this' context
		nativeFunc := AsNativeFunction(fn)
		prevThis := vm.currentThis
		vm.currentThis = thisValue
		defer func() { vm.currentThis = prevThis }()
		return nativeFunc.Fn(args)

	case TypeNativeFunctionWithProps:
		// Handle native function with properties
		nativeFuncWithProps := fn.AsNativeFunctionWithProps()
		prevThis := vm.currentThis
		vm.currentThis = thisValue
		defer func() { vm.currentThis = prevThis }()
		return nativeFuncWithProps.Fn(args)

	case TypeClosure, TypeFunction:
		// For user-defined functions, use the sentinel safe execution path which
		// integrates correctly with the interpreter loop and ensures exceptions
		// are surfaced as ExceptionError without corrupting VM state.
		return vm.executeUserFunctionSafe(fn, thisValue, args)

	case TypeBoundFunction:
		// Handle bound functions by delegating to the original function
		boundFunc := fn.AsBoundFunction()
		// Combine partial args with call-time args
		finalArgs := make([]Value, len(boundFunc.PartialArgs)+len(args))
		copy(finalArgs, boundFunc.PartialArgs)
		copy(finalArgs[len(boundFunc.PartialArgs):], args)
		// Use the bound 'this' value
		return vm.Call(boundFunc.OriginalFunction, boundFunc.BoundThis, finalArgs)

	default:
		return Undefined, fmt.Errorf("cannot call non-function value of type %v", fn.Type())
	}
}

// executeUserFunctionSafe executes a user function from a native function using sentinel frames
// This allows proper nested calls without infinite recursion
func (vm *VM) executeUserFunctionSafe(fn Value, thisValue Value, args []Value) (Value, error) {
	// Set up the caller context first
	callerRegisters := make([]Value, 1)
	destReg := byte(0)
	callerIP := 0

	// Add a sentinel frame that will cause vm.run() to return when it hits this frame
	sentinelFrame := &vm.frames[vm.frameCount]
	sentinelFrame.isSentinelFrame = true
	sentinelFrame.closure = nil               // Sentinel frames don't have closures
	sentinelFrame.targetRegister = destReg    // Target register in caller
	sentinelFrame.registers = callerRegisters // Give it the caller registers for the result
	vm.frameCount++

	// Use prepareCall to set up the function call
	shouldSwitch, err := vm.prepareCall(fn, thisValue, args, destReg, callerRegisters, callerIP)
	if err != nil {
		// Remove sentinel frame on error
		vm.frameCount--
		return Undefined, err
	}

	if !shouldSwitch {
		// Native function was executed directly
		// Remove sentinel frame
		vm.frameCount--
		return callerRegisters[destReg], nil
	}

	// We have a new frame set up, mark it as direct call
	if vm.frameCount > 1 { // frameCount includes the sentinel frame
		vm.frames[vm.frameCount-1].isDirectCall = true
	}

	// Execute the VM run loop - it will return when it hits the sentinel frame
	status, result := vm.run()

	if status == InterpretRuntimeError {
		// If the VM is unwinding an exception, surface it as an ExceptionError
		if vm.unwinding && vm.currentException != Null {
			ex := vm.currentException
			// Clear VM exception state before returning to native caller, since
			// we are handing the exception over as a Go error for the native to handle.
			vm.currentException = Null
			vm.unwinding = false
			return Undefined, exceptionError{exception: ex}
		}
		return Undefined, fmt.Errorf("runtime error during user function execution")
	}
	// If we reached a direct-call boundary and returned without InterpretRuntimeError,
	// propagate any pending exception to the native caller.
	if vm.unwinding && vm.currentException != Null {
		ex := vm.currentException
		vm.currentException = Null
		vm.unwinding = false
		return Undefined, exceptionError{exception: ex}
	}

	return result, nil
}

// ExecuteGenerator is the public interface for generator execution
func (vm *VM) ExecuteGenerator(genObj *GeneratorObject, sentValue Value) (Value, error) {
	return vm.executeGenerator(genObj, sentValue)
}

// ExecuteGeneratorWithException is the public interface for generator execution with exception injection
func (vm *VM) ExecuteGeneratorWithException(genObj *GeneratorObject, exception Value) (Value, error) {
	return vm.executeGeneratorWithException(genObj, exception)
}

// NewExceptionError creates an ExceptionError from a VM Value for use in builtins.
func (vm *VM) NewExceptionError(value Value) error {
	return exceptionError{exception: value}
}
