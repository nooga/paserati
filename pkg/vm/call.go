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
			if argCount != calleeFunc.Arity {
				currentFrame.ip = callerIP
				return false, fmt.Errorf("Expected %d arguments but got %d", calleeFunc.Arity, argCount)
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
		newFrame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]
		vm.nextRegSlot += requiredRegs
		
		// Copy fixed arguments
		for i := 0; i < calleeFunc.Arity && i < argCount; i++ {
			if i < len(newFrame.registers) {
				newFrame.registers[i] = args[i]
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
		
		// Arity checking for native functions
		if nativeFunc.Arity >= 0 {
			if nativeFunc.Variadic {
				if argCount < nativeFunc.Arity {
					currentFrame.ip = callerIP
					return false, fmt.Errorf("Native function expected at least %d arguments but got %d", nativeFunc.Arity, argCount)
				}
			} else {
				if argCount != nativeFunc.Arity {
					currentFrame.ip = callerIP
					return false, fmt.Errorf("Native function expected %d arguments but got %d", nativeFunc.Arity, argCount)
				}
			}
		}
		
		// Native functions execute immediately in caller's context
		result := nativeFunc.Fn(args)
		
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
				if argCount != nativeFuncWithProps.Arity {
					currentFrame.ip = callerIP
					return false, fmt.Errorf("Native function expected %d arguments but got %d", nativeFuncWithProps.Arity, argCount)
				}
			}
		}
		
		// Execute immediately
		result := nativeFuncWithProps.Fn(args)
		
		// Store result
		if int(destReg) < len(callerRegisters) {
			callerRegisters[destReg] = result
		} else {
			currentFrame.ip = callerIP
			return false, fmt.Errorf("Internal Error: Invalid destination register %d", destReg)
		}
		
		return false, nil
		
	default:
		currentFrame.ip = callerIP
		return false, fmt.Errorf("Cannot call non-function value of type %v", calleeVal.Type())
	}
}

// prepareMethodCall is a convenience wrapper for method calls that handles 'this' binding
func (vm *VM) prepareMethodCall(calleeVal Value, thisValue Value, args []Value, destReg byte, callerRegisters []Value, callerIP int) (bool, error) {
	// TODO: Currently native functions don't receive 'this' as first parameter
	// This is a known limitation - see TODO comment in original OpCallMethod
	// For now, just pass the args as-is for native functions
	
	// For all function types, pass thisValue for frame setup but don't modify args
	return vm.prepareCall(calleeVal, thisValue, args, destReg, callerRegisters, callerIP)
}