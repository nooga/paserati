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

	// Execute the async function body SYNCHRONOUSLY until first await
	// Per ECMAScript spec, the body runs synchronously until the first await expression
	// Only the continuation after await is scheduled as a microtask
	result, err := vm.executeAsyncFunctionBody(calleeVal, thisValue, args, promiseObj)

	if err != nil {
		// Reject the promise with the error
		if debugAsyncAwait {
			funcName := "?"
			if calleeVal.Type() == TypeClosure {
				funcName = calleeVal.AsClosure().Fn.Name
			} else if calleeVal.Type() == TypeFunction {
				funcName = calleeVal.AsFunction().Name
			}
			fmt.Printf("[ASYNC-DEBUG] func=%s ERROR: %v\n", funcName, err)
		}
		// Reject with the original exception value (not stringified) so catch handlers
		// receive the actual Error object, not a string representation
		if exErr, ok := err.(exceptionError); ok {
			vm.rejectPromise(promiseObj, exErr.exception)
		} else {
			vm.rejectPromise(promiseObj, NewString(err.Error()))
		}
	} else if promiseObj.Frame != nil {
		// Function hit an await and is suspended (Frame is set by OpAwait)
		if debugAsyncAwait {
			funcName := "?"
			if calleeVal.Type() == TypeClosure {
				funcName = calleeVal.AsClosure().Fn.Name
			} else if calleeVal.Type() == TypeFunction {
				funcName = calleeVal.AsFunction().Name
			}
			fmt.Printf("[ASYNC-DEBUG] func=%s SUSPENDED at await\n", funcName)
		}
	} else {
		// Function completed synchronously without hitting await
		if debugAsyncAwait {
			funcName := "?"
			if calleeVal.Type() == TypeClosure {
				funcName = calleeVal.AsClosure().Fn.Name
			} else if calleeVal.Type() == TypeFunction {
				funcName = calleeVal.AsFunction().Name
			}
			fmt.Printf("[ASYNC-DEBUG] func=%s completed sync, result=%s\n", funcName, result.Inspect())
		}
		vm.resolvePromise(promiseObj, result)
	}

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

	// Save current VM state so we can restore after vm.run() returns
	savedFrameCount := vm.frameCount
	savedNextRegSlot := vm.nextRegSlot

	// Set up caller context for sentinel frame approach
	callerRegisters := make([]Value, 1)
	destReg := byte(0)

	// Add a sentinel frame that will cause vm.run() to return when async function yields/returns
	sentinelFrame := &vm.frames[vm.frameCount]
	sentinelFrame.isSentinelFrame = true
	sentinelFrame.closure = nil               // Sentinel frames don't have closures
	sentinelFrame.openUpvalues = nil          // Never captures; clear any stale head from prior slot use
	sentinelFrame.targetRegister = destReg    // Target register in caller
	sentinelFrame.registers = callerRegisters // Give it the caller registers for the result
	vm.frameCount++

	// Check if we have space for the async function frame
	if vm.frameCount >= len(vm.frames) {
		vm.frameCount = savedFrameCount // Restore
		return Undefined, fmt.Errorf("Stack overflow")
	}

	// Allocate registers for the async function
	regSize := funcObj.RegisterSize
	if vm.nextRegSlot+regSize > len(vm.registerStack) {
		vm.frameCount = savedFrameCount // Restore
		return Undefined, fmt.Errorf("Out of registers")
	}

	// Set up the async function frame
	frame := &vm.frames[vm.frameCount]
	frame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+regSize]
	frame.allocatedRegSize = regSize // Track actual allocation for proper cleanup
	frame.ip = 0                     // Start from beginning
	frame.targetRegister = destReg
	frame.thisValue = thisValue
	frame.homeObject = funcObj.HomeObject // Set [[HomeObject]] for super property access (object literal methods)
	frame.isConstructorCall = false
	frame.isDirectCall = true      // Mark as direct call for proper return handling
	frame.isSentinelFrame = false  // Clear sentinel flag - this frame slot may have been a sentinel in a previous call
	frame.promiseObj = promiseObj  // Link frame to promise object - critical for OpAwait!
	frame.generatorObj = nil       // Clear generator object when reusing frame
	// frame.args is the field OpGetArguments actually reads to build the
	// `arguments` object (frame.argCount only sizes the register-copy loop
	// below) - see its "args were copied when the frame was created in
	// prepareCall" comment in vm.go. prepareCall (call.go) and every other
	// frame-setup path sets this; this hand-built one previously didn't, so
	// it stayed at whatever `frame.args` slice (or nil) was left by the last
	// call to reuse this physical vm.frames[N] slot - independent of the
	// correctly-set argCount/registers, so an async function's own
	// `arguments.length` always read as if called with the *previous*
	// occupant's argument count (typically 0, since a freshly-extended
	// vm.frames backing array zero-initializes every field). Set from the
	// same `args` this function already copies into registers, before
	// anything could mutate it.
	frame.args = args
	// Store original callee for arguments.callee, mirroring prepareCall.
	// calleeVal is this function's own top-level parameter, not
	// necessarily the same value prepareCallWithGeneratorMode's two separate
	// calleeVal/originalCallee parameters would use on every path - verified
	// directly for both a plain call and a bound call (arguments.callee must
	// name the original target, not the bound wrapper) in
	// pkg/driver/async_arguments_length_test.go's TestAsyncFunctionArgumentsCallee.
	frame.calleeValue = calleeVal
	// Clear any stale cached `arguments` object left behind by whatever call
	// last occupied this frame slot, for the same reuse-by-index reason as
	// openUpvalues below: every other fresh-frame setup (prepareCall,
	// op_spreadnew.go, every resumeGenerator*/resumeAsyncFunction* resume)
	// explicitly resets this to Undefined, and this hand-built setup is the
	// one path that historically didn't.
	frame.argumentsObject = Undefined
	// Clear any stale open-upvalue chain left behind by whatever call last
	// occupied this frame slot. Every other fresh-frame setup does this
	// (prepareCall's regular path, every sentinelFrame reuse, every
	// resumeGenerator*/resumeAsyncFunction* resume) - this was the one path
	// that didn't, because an async function's *initial* synchronous call
	// (as opposed to a resumption) builds its frame by hand here rather than
	// going through prepareCall. Left uncleared, a still-open upvalue chain
	// from a DIFFERENT, still-suspended async instance that previously used
	// this same physical vm.frames[N] slot gets silently prepended onto by
	// this instance's own captureUpvalue calls (harmless address-wise, since
	// relocateOpenUpvalues correctly skips entries outside its own register
	// window) - but when THIS instance later returns normally, closing its
	// own (contaminated) chain via closeFrameUpvalues also closes the OTHER,
	// unrelated instance's still-live upvalues at whatever stale value they
	// held at that moment, permanently freezing that other instance's
	// closures out from ever observing its own later writes. Reproduced with
	// two concurrent async functions on different setTimeout delays, each
	// with a local captured by a closure: the earlier-scheduled one's
	// closure got stuck reading its pre-suspend value forever.
	frame.openUpvalues = nil

	if closureObj != nil {
		frame.closure = closureObj
	} else {
		// Create a temporary closure for the function
		closureVal := NewClosure(funcObj, nil)
		frame.closure = closureVal.AsClosure()
	}

	// Allocate spill slots if this function needs them (for register overflow)
	if funcObj.Chunk.NumSpillSlots > 0 {
		frame.spillSlots = make([]Value, funcObj.Chunk.NumSpillSlots)
	} else {
		frame.spillSlots = nil
	}

	// Zero out all registers first to prevent stale data from previous calls
	// that used the same register stack region
	for i := range frame.registers {
		frame.registers[i] = Undefined
	}

	// Set up arguments in registers. Mirrors prepareCall's argument-copying
	// and variadic-handling logic specifically (call.go), not its full frame
	// setup - this path builds its frame by hand instead of going through
	// prepareCall (see the openUpvalues comment above for why), so it must
	// independently reproduce prepareCall's variadic handling too.
	// It previously didn't: a rest-only async function (e.g. `async
	// function f(...args) {}`) called with fewer arguments than its arity
	// left the rest-parameter register at its zeroed Undefined default
	// instead of an empty array, since nothing here ever checked
	// funcObj.Variadic. `await f()` observed `undefined` where `[]` was
	// expected.
	argCount := len(args)
	frame.argCount = argCount
	maxArgsToCopy := argCount
	if funcObj.Arity > maxArgsToCopy {
		maxArgsToCopy = funcObj.Arity
	}
	if maxArgsToCopy > len(frame.registers) {
		maxArgsToCopy = len(frame.registers)
	}
	for i := 0; i < maxArgsToCopy; i++ {
		if i < argCount {
			frame.registers[i] = args[i]
		} else {
			frame.registers[i] = Undefined
		}
	}

	if funcObj.Variadic {
		extraArgCount := argCount - funcObj.Arity
		var restArray Value
		if extraArgCount <= 0 {
			restArray = NewArray()
		} else {
			restArray = NewArray()
			restArrayObj := restArray.AsArray()
			for i := 0; i < extraArgCount; i++ {
				argIndex := funcObj.Arity + i
				if argIndex < len(args) {
					restArrayObj.Append(args[argIndex])
				}
			}
		}
		if funcObj.Arity < len(frame.registers) {
			frame.registers[funcObj.Arity] = restArray
		}
	}

	// Update VM state
	vm.frameCount++
	vm.nextRegSlot += regSize

	if debugAsyncAwait {
		fmt.Printf("[ASYNC-BODY] func=%s starting execution, regSize=%d, args=%d, frameCount=%d, nextRegSlot=%d\n",
			funcObj.Name, regSize, len(args), vm.frameCount, vm.nextRegSlot)
		for i := 0; i < len(frame.registers) && i < 5; i++ {
			fmt.Printf("[ASYNC-BODY]   R%d = %s\n", i, frame.registers[i].Inspect())
		}
	}

	// Execute the VM run loop - it will return when the async function yields or completes
	status, result := vm.run()

	// CRITICAL: Clean up frames only if OpAwait suspended execution
	// When the async function completes normally via OpReturn, the sentinel frame
	// is already cleaned up by OpReturn. But when execution suspends at OpAwait,
	// the frames are NOT cleaned up, so we need to restore them here.
	// We can detect this by checking if frameCount is still higher than what we saved.
	if vm.frameCount > savedFrameCount {
		vm.frameCount = savedFrameCount
		vm.nextRegSlot = savedNextRegSlot
	}

	if status == InterpretRuntimeError {
		if vm.unwinding && vm.currentException != Null {
			exc := vm.currentException
			// CRITICAL: Clear exception state so it doesn't leak to the caller's vm.run().
			// The exception is captured in the returned error and will be used to reject
			// the async function's promise. Without this, the caller's OpCall handler
			// sees stale unwinding/crossedNative flags and returns InterpretRuntimeError
			// from the wrong vm.run() invocation.
			vm.currentException = Null
			vm.unwinding = false
			vm.unwindingCrossedNative = false
			return Undefined, exceptionError{exception: exc}
		}
		return Undefined, fmt.Errorf("runtime error during async function execution")
	}

	return result, nil
}
