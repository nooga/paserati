package vm

import (
	"fmt"
	"unsafe"
)

// executeAsyncFunction creates a Promise and starts executing an async function
// The execution will be suspended at await points and resumed via the async runtime
func (vm *VM) executeAsyncFunction(calleeVal Value, thisValue Value, args []Value) Value {
	// Create a new pending Promise
	// Use the same pattern as in promise.go
	baseObj := NewObject(vm.PromisePrototype).AsPlainObject()
	promiseObj := &PromiseObject{
		Object:           baseObj.Object,
		State:            PromisePending,
		Result:           Undefined,
		FulfillReactions: []PromiseReaction{},
		RejectReactions:  []PromiseReaction{},
	}
	promiseVal := Value{typ: TypePromise, obj: unsafe.Pointer(promiseObj)}

	// Schedule the async function execution as a microtask
	asyncRuntime := vm.GetAsyncRuntime()

	asyncRuntime.ScheduleMicrotask(func() {
		// Execute the async function
		// We need to set up a special frame that knows about the promise
		result, err := vm.executeAsyncFunctionBody(calleeVal, thisValue, args, promiseObj)

		if err != nil {
			// Reject the promise with the error
			vm.rejectPromise(promiseObj, NewString(err.Error()))
		} else {
			// Resolve the promise with the result
			vm.resolvePromise(promiseObj, result)
		}
	})

	return promiseVal
}

// executeAsyncFunctionBody executes the body of an async function
// Uses sentinel frame approach like generators to isolate execution
func (vm *VM) executeAsyncFunctionBody(calleeVal Value, thisValue Value, args []Value, promiseObj *PromiseObject) (Value, error) {
	// Store promise object and function reference for later resumption
	promiseObj.Function = calleeVal
	promiseObj.ThisValue = thisValue

	// Extract function object
	var funcObj *FunctionObject
	var closureObj *ClosureObject

	if calleeVal.Type() == TypeFunction {
		funcObj = calleeVal.AsFunction()
	} else if calleeVal.Type() == TypeClosure {
		closureObj = calleeVal.AsClosure()
		funcObj = closureObj.Fn
	} else {
		return Undefined, fmt.Errorf("Invalid async function type")
	}

	// Set up caller context for sentinel frame approach
	callerRegisters := make([]Value, 1)
	destReg := byte(0)

	// Add a sentinel frame that will cause vm.run() to return when async function yields/returns
	sentinelFrame := &vm.frames[vm.frameCount]
	sentinelFrame.isSentinelFrame = true
	sentinelFrame.closure = nil               // Sentinel frames don't have closures
	sentinelFrame.targetRegister = destReg    // Target register in caller
	sentinelFrame.registers = callerRegisters // Give it the caller registers for the result
	vm.frameCount++

	// Check if we have space for the async function frame
	if vm.frameCount >= MaxFrames {
		vm.frameCount-- // Remove sentinel frame
		return Undefined, fmt.Errorf("Stack overflow")
	}

	// Allocate registers for the async function
	regSize := funcObj.RegisterSize
	if vm.nextRegSlot+regSize > len(vm.registerStack) {
		vm.frameCount-- // Remove sentinel frame
		return Undefined, fmt.Errorf("Out of registers")
	}

	// Set up the async function frame
	frame := &vm.frames[vm.frameCount]
	frame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+regSize]
	frame.ip = 0 // Start from beginning
	frame.targetRegister = destReg
	frame.thisValue = thisValue
	frame.isConstructorCall = false
	frame.isDirectCall = true      // Mark as direct call for proper return handling
	frame.promiseObj = promiseObj  // Link frame to promise object - critical for OpAwait!

	if closureObj != nil {
		frame.closure = closureObj
	} else {
		// Create a temporary closure for the function
		closureVal := NewClosure(funcObj, nil)
		frame.closure = closureVal.AsClosure()
	}

	// Set up arguments in registers
	frame.argCount = len(args)
	for i, arg := range args {
		if i < len(frame.registers) {
			frame.registers[i] = arg
		}
	}

	// Update VM state
	vm.frameCount++
	vm.nextRegSlot += regSize

	// Execute the VM run loop - it will return when the async function yields or completes
	status, result := vm.run()

	if status == InterpretRuntimeError {
		if vm.unwinding && vm.currentException != Null {
			return Undefined, exceptionError{exception: vm.currentException}
		}
		return Undefined, fmt.Errorf("runtime error during async function execution")
	}

	return result, nil
}
