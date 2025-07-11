package vm

import (
	"fmt"
	"unsafe"
)

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
	argCount := len(args)
	currentFrame := &vm.frames[vm.frameCount-1]

	// fmt.Printf("DEBUG prepareCall: callee=%v, calleeType=%v, this=%v, args=%v\n",
	// 	calleeVal.Inspect(), calleeVal.Type(), thisValue.Inspect(), len(args))

	switch calleeVal.Type() {
	case TypeClosure:
		calleeClosure := AsClosure(calleeVal)
		calleeFunc := calleeClosure.Fn

		// Arity checking
		if calleeFunc.Variadic {
			if argCount < calleeFunc.Arity {
				currentFrame.ip = callerIP
				return false, fmt.Errorf("Expected at least %d arguments but got %d", calleeFunc.Arity, argCount)
			}
		} else {
			// Allow fewer arguments for functions with optional parameters
			// The compiler handles padding with undefined for missing optional parameters
			if argCount > calleeFunc.Arity {
				currentFrame.ip = callerIP
				return false, fmt.Errorf("Expected at most %d arguments but got %d", calleeFunc.Arity, argCount)
			}
		}

		// Check frame limit
		if vm.frameCount == MaxFrames {
			currentFrame.ip = callerIP
			return false, fmt.Errorf("Stack overflow")
		}

		// Check register stack space
		requiredRegs := calleeFunc.RegisterSize
		if vm.nextRegSlot+requiredRegs > len(vm.registerStack) {
			currentFrame.ip = callerIP
			return false, fmt.Errorf("Register stack overflow")
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
		newFrame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]
		vm.nextRegSlot += requiredRegs

		// Copy fixed arguments and initialize missing ones with undefined
		for i := 0; i < calleeFunc.Arity; i++ {
			if i < len(newFrame.registers) {
				if i < argCount {
					newFrame.registers[i] = args[i]
				} else {
					newFrame.registers[i] = Undefined
				}
			} else {
				// Rollback and error
				vm.nextRegSlot -= requiredRegs
				currentFrame.ip = callerIP
				return false, fmt.Errorf("Internal Error: Argument register index out of bounds")
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
		return vm.prepareCall(closureVal, thisValue, args, destReg, callerRegisters, callerIP)

	case TypeNativeFunction:
		nativeFunc := AsNativeFunction(calleeVal)

		// fmt.Printf("DEBUG prepareCall: nativeFunc=%v, nativeFunc.Arity=%v, nativeFunc.Variadic=%v, argCount=%v\n",
		// 	nativeFunc.Name, nativeFunc.Arity, nativeFunc.Variadic, argCount)

		// Arity checking for native functions
		if nativeFunc.Arity >= 0 {
			if nativeFunc.Variadic {
				if argCount < nativeFunc.Arity {
					currentFrame.ip = callerIP
					return false, fmt.Errorf("Native function expected at least %d arguments but got %d", nativeFunc.Arity, argCount)
				}
			} else {
				// For non-variadic functions, allow fewer arguments if they might have optional parameters
				// This is a pragmatic fix for cases where the compiler doesn't properly pad optional parameters
				if argCount > nativeFunc.Arity {
					currentFrame.ip = callerIP
					return false, fmt.Errorf("Native function expected %d arguments but got %d", nativeFunc.Arity, argCount)
				}
				// Allow fewer arguments - the native function implementation should handle undefined parameters
			}
		}

		//fmt.Printf("DEBUG prepareCall: args=%v\n", args)
		// Set the current 'this' value for native function access
		vm.currentThis = thisValue
		// Native functions execute immediately in caller's context
		result, err := nativeFunc.Fn(args)
		if err != nil {
			// Return error to be handled by the VM
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

		// Arity checking
		if nativeFuncWithProps.Arity >= 0 {
			if nativeFuncWithProps.Variadic {
				if argCount < nativeFuncWithProps.Arity {
					currentFrame.ip = callerIP
					return false, fmt.Errorf("Native function expected at least %d arguments but got %d", nativeFuncWithProps.Arity, argCount)
				}
			} else {
				// For non-variadic functions, allow fewer arguments if they might have optional parameters
				// This is a pragmatic fix for cases where the compiler doesn't properly pad optional parameters
				if argCount > nativeFuncWithProps.Arity {
					currentFrame.ip = callerIP
					return false, fmt.Errorf("Native function expected %d arguments but got %d", nativeFuncWithProps.Arity, argCount)
				}
				// Allow fewer arguments - the native function implementation should handle undefined parameters
			}
		}

		// Set the current 'this' value for native function access
		vm.currentThis = thisValue
		// Execute immediately
		result, err := nativeFuncWithProps.Fn(args)
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
				if argCount > asyncNativeFunc.Arity {
					currentFrame.ip = callerIP
					return false, fmt.Errorf("Async native function expected %d arguments but got %d", asyncNativeFunc.Arity, argCount)
				}
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
		fmt.Printf("DEBUG: Trying to call non-function value:\n")
		fmt.Printf("  Type: %v\n", calleeVal.Type())
		fmt.Printf("  Value: %s\n", calleeVal.Inspect())
		fmt.Printf("  Raw: %+v\n", calleeVal)
		return false, fmt.Errorf("Cannot call non-function value of type %v", calleeVal.Type())
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
