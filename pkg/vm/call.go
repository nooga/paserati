package vm

import (
	"fmt"
	"os"
	"unsafe"
)

// ExceptionError is an interface for errors that should be thrown as VM exceptions
type ExceptionError interface {
	error
	GetExceptionValue() Value
}

// exceptionError is a concrete implementation used to propagate VM exceptions
// across native boundaries as Go errors while preserving the original Value.
type exceptionError struct {
	exception Value
}

func (e exceptionError) Error() string {
	return "VM exception"
}

func (e exceptionError) GetExceptionValue() Value {
	return e.exception
}

// prepareCall sets up a function call and returns whether the interpreter should switch to the new frame.
// For native functions, it executes immediately and returns false.
// For closures/functions, it sets up the frame and returns true to switch context.
//
// Parameters:
//   - calleeVal: The function/closure to call
//   - thisValue: The 'this' context for the call (use Undefined for regular calls)
//   - args: The arguments to pass (can be a slice view of registers)
//   - destReg: Where to store the result in caller's registers
//   - callerRegisters: The caller's register array
//   - callerIP: The caller's instruction pointer (for error reporting and return)
//
// Returns (shouldSwitchFrame, error)
func (vm *VM) prepareCall(calleeVal Value, thisValue Value, args []Value, destReg byte, callerRegisters []Value, callerIP int) (bool, error) {
	return vm.prepareCallWithGeneratorMode(calleeVal, thisValue, args, destReg, callerRegisters, callerIP, false)
}

func (vm *VM) prepareCallWithGeneratorMode(calleeVal Value, thisValue Value, args []Value, destReg byte, callerRegisters []Value, callerIP int, isGeneratorExecution bool) (bool, error) {
	argCount := len(args)
	currentFrame := &vm.frames[vm.frameCount-1]

	// fmt.Printf("DEBUG prepareCall: callee=%v, calleeType=%v, this=%v, args=%v\n",
	// 	calleeVal.Inspect(), calleeVal.Type(), thisValue.Inspect(), len(args))

	// Handle Proxy with apply trap
	if calleeVal.Type() == TypeProxy {
		proxy := calleeVal.AsProxy()
		if proxy.Revoked {
			vm.runtimeError("Cannot call revoked Proxy")
			return false, nil
		}

		// Check for apply trap
		if applyTrap, ok := proxy.Handler().AsPlainObject().GetOwn("apply"); ok {
			// Validate trap is callable
			if !applyTrap.IsCallable() {
				vm.runtimeError("'apply' on proxy: trap is not a function")
				return false, nil
			}

			// Convert args to array for trap call
			argsArray := NewArray()
			arrObj := argsArray.AsArray()
			for _, arg := range args {
				arrObj.Append(arg)
			}

			// Call handler.apply(target, thisArg, argumentsList)
			trapArgs := []Value{proxy.Target(), thisValue, argsArray}
			result, err := vm.Call(applyTrap, proxy.Handler(), trapArgs)
			if err != nil {
				if ee, ok := err.(ExceptionError); ok {
					vm.throwException(ee.GetExceptionValue())
				} else {
					vm.runtimeError(err.Error())
				}
				return false, nil
			}

			// Store result in destination register
			callerRegisters[destReg] = result
			return false, nil // Don't switch frames, we handled the call
		}

		// No apply trap, delegate to target (call the target as a function)
		return vm.prepareCall(proxy.Target(), thisValue, args, destReg, callerRegisters, callerIP)
	}

	switch calleeVal.Type() {
	case TypeClosure:
		calleeClosure := AsClosure(calleeVal)
		calleeFunc := calleeClosure.Fn

		// Check if this is an async generator function (both flags set)
		if calleeFunc.IsAsync && calleeFunc.IsGenerator && !isGeneratorExecution {
			// Create an async generator object instead of calling the function
			genVal := NewAsyncGenerator(calleeVal)
			genObj := genVal.AsAsyncGenerator()

			// Set the generator's prototype according to ECMAScript spec:
			// Try to get the function's .prototype property
			prototypeVal := Undefined
			if calleeFunc.Properties != nil {
				if calleeFunc.Properties.HasOwn("prototype") {
					prototypeVal, _ = calleeFunc.Properties.GetOwn("prototype")
				}
			}

			// If .prototype is an object, use it as the generator's prototype
			// Otherwise, use the default AsyncGeneratorPrototype
			if prototypeVal.IsObject() && prototypeVal.Type() == TypeObject {
				genObj.Prototype = prototypeVal.AsPlainObject()
			} else {
				// Use default AsyncGeneratorPrototype
				if vm.AsyncGeneratorPrototype.Type() == TypeObject {
					genObj.Prototype = vm.AsyncGeneratorPrototype.AsPlainObject()
				}
			}

			// Store the arguments and 'this' value for when the generator starts
			genObj.Args = make([]Value, len(args))
			copy(genObj.Args, args)
			genObj.This = thisValue

			callerRegisters[destReg] = genVal
			return false, nil // Don't switch frames
		}

		// Check if this is a generator function (but skip if we're already executing a generator)
		if calleeFunc.IsGenerator && !isGeneratorExecution {
			// Create a generator object instead of calling the function
			genVal := NewGenerator(calleeVal)
			genObj := genVal.AsGenerator()

			// Set the generator's prototype according to ECMAScript spec:
			// Try to get the function's .prototype property
			prototypeVal := Undefined
			if calleeFunc.Properties != nil {
				if calleeFunc.Properties.HasOwn("prototype") {
					prototypeVal, _ = calleeFunc.Properties.GetOwn("prototype")
				}
			}

			// If .prototype is an object, use it as the generator's prototype
			// Otherwise, use the default GeneratorPrototype
			if prototypeVal.IsObject() && prototypeVal.Type() == TypeObject {
				genObj.Prototype = prototypeVal.AsPlainObject()
			} else {
				// Use default GeneratorPrototype
				if vm.GeneratorPrototype.Type() == TypeObject {
					genObj.Prototype = vm.GeneratorPrototype.AsPlainObject()
				}
			}

			// Store the arguments and 'this' value for when the generator starts
			// We'll need to pass these when ExecuteGenerator is called
			genObj.Args = make([]Value, len(args))
			copy(genObj.Args, args)
			genObj.This = thisValue

			callerRegisters[destReg] = genVal
			return false, nil // Don't switch frames
		}

		// Check if this is an async function - wrap execution in a Promise
		// Skip this if we're executing a generator (including async generators)
		if calleeFunc.IsAsync && !isGeneratorExecution {
			// Create a Promise and start async execution
			promiseVal := vm.executeAsyncFunction(calleeVal, thisValue, args)
			callerRegisters[destReg] = promiseVal
			return false, nil // Don't switch frames - async execution happens via microtasks
		}

		// Arity checking
		if calleeFunc.Variadic {
			if argCount < calleeFunc.Arity {
				currentFrame.ip = callerIP
				return false, fmt.Errorf("Expected at least %d arguments but got %d", calleeFunc.Arity, argCount)
			}
		} else {
			// Allow fewer arguments for functions with optional parameters
			// The compiler handles padding with undefined for missing optional parameters
			// Allow extra arguments (JavaScript behavior) - they are ignored or available via arguments object
		}

		// Check frame limit
		if vm.frameCount == MaxFrames {
			currentFrame.ip = callerIP
			trace := vm.CaptureStackTrace()
			fmt.Printf("\n=== VM Stack (overflow) ===\n%s\n===========================\n", trace)
			return false, fmt.Errorf("Stack overflow\nStack: %s", trace)
		}

		// Check register stack space
		requiredRegs := calleeFunc.RegisterSize
		if vm.nextRegSlot+requiredRegs > len(vm.registerStack) {
			currentFrame.ip = callerIP
			trace := vm.CaptureStackTrace()
			fmt.Printf("\n=== VM Stack (register overflow) ===\n%s\n====================================\n", trace)
			return false, fmt.Errorf("Register stack overflow\nStack: %s", trace)
		}

		// Store return IP in current frame
		currentFrame.ip = callerIP

		// Set up new frame
		newFrame := &vm.frames[vm.frameCount]
		newFrame.closure = calleeClosure
		newFrame.ip = 0
		newFrame.targetRegister = destReg
		newFrame.thisValue = thisValue
		newFrame.isConstructorCall = false
		newFrame.isDirectCall = false
		newFrame.isSentinelFrame = false // Clear sentinel flag when reusing frame
		newFrame.generatorObj = nil      // Clear generator object when reusing frame
		newFrame.argCount = argCount     // Store actual argument count for arguments object
		// Copy arguments for arguments object (before registers get mutated by function execution)
		newFrame.args = make([]Value, argCount)
		copy(newFrame.args, args)
		newFrame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]
		vm.nextRegSlot += requiredRegs

		// Copy arguments to registers
		// We need to copy ALL passed arguments (up to argCount), not just up to Arity,
		// so that the arguments object can access them via OpGetArguments.
		// However, we can only copy as many as fit in the allocated registers.
		maxArgsToCopy := argCount
		if calleeFunc.Arity > maxArgsToCopy {
			maxArgsToCopy = calleeFunc.Arity
		}
		if maxArgsToCopy > len(newFrame.registers) {
			maxArgsToCopy = len(newFrame.registers)
		}

		for i := 0; i < maxArgsToCopy; i++ {
			if i < argCount {
				newFrame.registers[i] = args[i]
			} else {
				newFrame.registers[i] = Undefined
			}
		}

		// Handle rest parameters for variadic functions
		if calleeFunc.Variadic {
			extraArgCount := argCount - calleeFunc.Arity
			var restArray Value

			if extraArgCount == 0 {
				restArray = vm.emptyRestArray
			} else {
				restArray = NewArray()
				restArrayObj := restArray.AsArray()
				for i := 0; i < extraArgCount; i++ {
					argIndex := calleeFunc.Arity + i
					if argIndex < len(args) {
						restArrayObj.Append(args[argIndex])
					}
				}
			}

			// Store rest array at the appropriate position
			if calleeFunc.Arity < len(newFrame.registers) {
				newFrame.registers[calleeFunc.Arity] = restArray
			}
		}

		// Initialize named function expression binding if present
		// For named function expressions like: let f = function g() { g(); }
		// The name 'g' should be accessible inside and refer to the closure itself
		if calleeFunc.NameBindingRegister >= 0 && calleeFunc.NameBindingRegister < len(newFrame.registers) {
			newFrame.registers[calleeFunc.NameBindingRegister] = calleeVal
		}

		// Frame is ready, tell interpreter to switch to it
		vm.frameCount++
		return true, nil

	case TypeFunction:
		// Convert bare function to closure
		funcToCall := AsFunction(calleeVal)
		tempClosure := &ClosureObject{
			Fn:       funcToCall,
			Upvalues: []*Upvalue{},
		}
		closureVal := Value{typ: TypeClosure, obj: unsafe.Pointer(tempClosure)}
		return vm.prepareCallWithGeneratorMode(closureVal, thisValue, args, destReg, callerRegisters, callerIP, isGeneratorExecution)

	case TypeNativeFunction:
		nativeFunc := AsNativeFunction(calleeVal)

		// fmt.Printf("DEBUG prepareCall: nativeFunc=%v, nativeFunc.Arity=%v, nativeFunc.Variadic=%v, argCount=%v\n",
		// 	nativeFunc.Name, nativeFunc.Arity, nativeFunc.Variadic, argCount)

		// Arity checking for native functions: be permissive like JS (missing args become undefined)
		// Only enforce minimum when variadic and declared arity > 0
		// Do not error for 0-arg constructors called without args

		//fmt.Printf("DEBUG prepareCall: args=%v\n", args)
		if debugCalls {
			fmt.Printf("[DEBUG call.go] Calling native function %s, frameCount=%d\n", nativeFunc.Name, vm.frameCount)
		}
		// Set the current 'this' value for native function access and restore after call
		oldThis := vm.currentThis
		vm.currentThis = thisValue
		// Native functions execute immediately in caller's context
		result, err := nativeFunc.Fn(args)
		vm.currentThis = oldThis
		if debugCalls {
			fmt.Printf("[DEBUG call.go] Native function %s returned, err=%v, frameCount=%d, unwinding=%v\n",
				nativeFunc.Name, err != nil, vm.frameCount, vm.unwinding)
		}

		if err != nil {
			// Always return the error to the caller; VM will handle conversion at the call site
			return false, err
		}

		//fmt.Printf("DEBUG prepareCall: result=%v\n", result.Inspect())

		// Store result
		if int(destReg) < len(callerRegisters) {
			callerRegisters[destReg] = result
		} else {
			currentFrame.ip = callerIP
			return false, fmt.Errorf("Internal Error: Invalid destination register %d", destReg)
		}

		// Native function completed, no frame switch needed
		return false, nil

	case TypeNativeFunctionWithProps:
		// Handle native function with properties
		nativeFuncWithProps := calleeVal.AsNativeFunctionWithProps()

		// Arity checking (permissive)

		// Set the current 'this' value for native function access and restore after call
		oldThis := vm.currentThis
		vm.currentThis = thisValue
		// Execute immediately
		result, err := nativeFuncWithProps.Fn(args)
		vm.currentThis = oldThis
		if err != nil {
			// Return error to be handled by the VM
			return false, err
		}

		// Store result
		if int(destReg) < len(callerRegisters) {
			callerRegisters[destReg] = result
		} else {
			currentFrame.ip = callerIP
			return false, fmt.Errorf("Internal Error: Invalid destination register %d", destReg)
		}

		return false, nil

	case TypeAsyncNativeFunction:
		// Handle async native function that can call bytecode
		asyncNativeFunc := calleeVal.AsAsyncNativeFunction()

		// Arity checking
		if asyncNativeFunc.Arity >= 0 {
			if asyncNativeFunc.Variadic {
				if argCount < asyncNativeFunc.Arity {
					currentFrame.ip = callerIP
					return false, fmt.Errorf("Async native function expected at least %d arguments but got %d", asyncNativeFunc.Arity, argCount)
				}
			} else {
				// For non-variadic functions, allow fewer arguments if they might have optional parameters
				// This is a pragmatic fix for cases where the compiler doesn't properly pad optional parameters
				// Allow extra arguments (JavaScript behavior) - they are ignored by the native function
				// Allow fewer arguments - the native function implementation should handle undefined parameters
			}
		}

		// Execute async native function
		_, err := vm.executeAsyncNativeFunction(asyncNativeFunc, args, destReg, callerRegisters)
		if err != nil {
			currentFrame.ip = callerIP
			return false, err
		}

		// Result already stored by executeAsyncNativeFunction
		return false, nil

	case TypeBoundFunction:
		// Handle bound function - delegate to original function with bound 'this' and combined args
		boundFunc := calleeVal.AsBoundFunction()

		// Combine partial args with call-time args
		finalArgs := make([]Value, len(boundFunc.PartialArgs)+len(args))
		copy(finalArgs, boundFunc.PartialArgs)
		copy(finalArgs[len(boundFunc.PartialArgs):], args)

		// Call the original function with the bound 'this' value (ignore the provided thisValue)
		return vm.prepareCall(boundFunc.OriginalFunction, boundFunc.BoundThis, finalArgs, destReg, callerRegisters, callerIP)

	default:
		currentFrame.ip = callerIP
		// Throw a TypeError exception for non-callable values
		errorMsg := fmt.Sprintf("%s is not a function", calleeVal.TypeName())

		// DEBUG: Add extra context to help identify what's undefined
		if calleeVal.Type() == TypeUndefined {
			// Try to provide more context by checking recent bytecode
			if debugCalls || true { // Temporarily always log undefined function calls
				fmt.Fprintf(os.Stderr, "[DEBUG call.go] Attempting to call undefined value\n")
				fmt.Fprintf(os.Stderr, "[DEBUG call.go] Frame count: %d\n", vm.frameCount)
				// Show last 10 frames to see the recursion pattern
				fmt.Fprintf(os.Stderr, "[DEBUG call.go] Last 10 frames:\n")
				start := vm.frameCount - 10
				if start < 0 {
					start = 0
				}
				for i := start; i < vm.frameCount; i++ {
					frame := &vm.frames[i]
					if frame.closure != nil && frame.closure.Fn != nil {
						fmt.Fprintf(os.Stderr, "  [%d] %s\n", i, frame.closure.Fn.Name)
					} else {
						fmt.Fprintf(os.Stderr, "  [%d] <unknown>\n", i)
					}
				}
				fmt.Fprintf(os.Stderr, "[DEBUG call.go] Stack trace:\n%s\n", vm.CaptureStackTrace())
			}
		}

		vm.ThrowTypeError(errorMsg)
		// Return false to indicate we're not switching frames (exception was thrown)
		return false, nil
	}
}

// prepareMethodCall is a convenience wrapper for method calls that handles 'this' binding
func (vm *VM) prepareMethodCall(calleeVal Value, thisValue Value, args []Value, destReg byte, callerRegisters []Value, callerIP int) (bool, error) {
	// Debug logging
	// fmt.Printf("DEBUG prepareMethodCall: callee=%v, calleeType=%v, this=%v, args=%v\n",
	// 	calleeVal.Inspect(), calleeVal.Type(), thisValue.Inspect(), len(args))

	// For all function types, pass thisValue for frame setup and let prepareCall handle the 'this' setting
	return vm.prepareCall(calleeVal, thisValue, args, destReg, callerRegisters, callerIP)
}

// prepareDirectCall is like prepareCall but sets the isDirectCall flag so the frame returns immediately
func (vm *VM) prepareDirectCall(calleeVal Value, thisValue Value, args []Value, destReg byte, callerRegisters []Value, callerIP int) (bool, error) {
	// First call the regular prepareCall
	shouldSwitch, err := vm.prepareCall(calleeVal, thisValue, args, destReg, callerRegisters, callerIP)

	// If we created a new frame, mark it as a direct call
	if shouldSwitch && vm.frameCount > 0 {
		vm.frames[vm.frameCount-1].isDirectCall = true
	}

	return shouldSwitch, err
}
