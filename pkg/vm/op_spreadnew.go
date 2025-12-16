package vm

import "fmt"

// handleOpSpreadNew handles OpSpreadNew bytecode instruction for constructor calls with spread arguments
func (vm *VM) handleOpSpreadNew(code []byte, ip *int, frame *CallFrame, registers []Value) (InterpretResult, Value) {
	destReg := code[*ip]
	constructorReg := code[*ip+1]
	spreadArgReg := code[*ip+2]
	*ip += 3

	callerRegisters := registers
	callerIP := *ip

	constructorVal := callerRegisters[constructorReg]

	// ES6 12.3.3.1.1 step 7: Validate that constructor is constructible
	// This must throw TypeError for primitives and non-constructor objects
	if !constructorVal.IsCallable() {
		frame.ip = callerIP
		vm.ThrowTypeError(fmt.Sprintf("%s is not a constructor", constructorVal.TypeName()))
		return InterpretRuntimeError, Undefined
	}

	// Additional check for functions that are not constructors
	// Arrow functions, async functions (non-generator), and plain generators cannot be constructors
	if constructorVal.Type() == TypeFunction {
		fn := AsFunction(constructorVal)
		if fn.IsArrowFunction || (fn.IsAsync && !fn.IsGenerator) {
			frame.ip = callerIP
			vm.ThrowTypeError(fmt.Sprintf("%s is not a constructor", constructorVal.TypeName()))
			return InterpretRuntimeError, Undefined
		}
	} else if constructorVal.Type() == TypeClosure {
		cl := AsClosure(constructorVal)
		if cl.Fn.IsArrowFunction || (cl.Fn.IsAsync && !cl.Fn.IsGenerator) {
			frame.ip = callerIP
			vm.ThrowTypeError(fmt.Sprintf("%s is not a constructor", constructorVal.TypeName()))
			return InterpretRuntimeError, Undefined
		}
	}

	spreadArrayVal := callerRegisters[spreadArgReg]

	// Extract arguments from spread array
	spreadArgs, err := vm.extractSpreadArguments(spreadArrayVal)
	if err != nil {
		frame.ip = callerIP
		// Check if it's a VM exception (TypeError, etc.) and propagate it
		if ee, ok := err.(ExceptionError); ok {
			vm.throwException(ee.GetExceptionValue())
			return InterpretRuntimeError, Undefined
		}
		// Otherwise wrap as generic runtime error
		status := vm.runtimeError("Spread constructor call error: %s", err.Error())
		return status, Undefined
	}
	argCount := len(spreadArgs)

	switch constructorVal.Type() {
	case TypeClosure:
		constructorClosure := AsClosure(constructorVal)
		constructorFunc := constructorClosure.Fn

		// Check if it's an arrow function
		if constructorFunc.IsArrowFunction {
			frame.ip = callerIP
			vm.ThrowTypeError("Arrow functions cannot be used as constructors")
			return InterpretRuntimeError, Undefined
		}

		// Check stack limits
		if vm.frameCount == MaxFrames {
			frame.ip = callerIP
			status := vm.runtimeError("Stack overflow during constructor call.")
			return status, Undefined
		}
		requiredRegs := constructorFunc.RegisterSize
		if vm.nextRegSlot+requiredRegs > len(vm.registerStack) {
			frame.ip = callerIP
			status := vm.runtimeError("Register stack overflow during constructor call.")
			return status, Undefined
		}

		// Determine the new.target value for this constructor call
		// If the caller is already a constructor (super() call), inherit its new.target
		// Otherwise, new.target is the constructor being called
		var newTargetValue Value
		if frame.isConstructorCall && frame.newTargetValue.Type() != TypeUndefined {
			// This is a super() call from a derived constructor - inherit new.target
			newTargetValue = frame.newTargetValue
		} else {
			// Direct new Constructor() call - new.target is the constructor
			newTargetValue = constructorVal
		}

		// Get the prototype to use for the instance from new.target.prototype
		// This ensures derived classes create instances with the correct prototype
		var instancePrototype Value
		if newTargetValue.Type() == TypeClosure {
			newTargetClosure := AsClosure(newTargetValue)
			newTargetFunc := newTargetClosure.Fn
			instancePrototype = newTargetFunc.getOrCreatePrototypeWithVM(vm)
		} else if newTargetValue.Type() == TypeFunction {
			newTargetFunc := AsFunction(newTargetValue)
			instancePrototype = newTargetFunc.getOrCreatePrototypeWithVM(vm)
		} else {
			// Fallback: use the constructor's prototype
			instancePrototype = constructorFunc.getOrCreatePrototypeWithVM(vm)
		}

		// Create instance (or leave undefined for derived constructors)
		var newInstance Value
		if constructorFunc.IsDerivedConstructor {
			newInstance = Undefined
		} else {
			newInstance = NewObject(instancePrototype)
		}

		frame.ip = callerIP

		// Create new frame
		newFrame := &vm.frames[vm.frameCount]
		newFrame.closure = constructorClosure
		newFrame.ip = 0
		newFrame.targetRegister = destReg
		newFrame.thisValue = newInstance
		newFrame.homeObject = instancePrototype  // Set [[HomeObject]] for super property access in constructors
		newFrame.isConstructorCall = true
		newFrame.isDirectCall = false            // Not a direct call (spread new)
		newFrame.isSentinelFrame = false         // Clear sentinel flag when reusing frame
		newFrame.newTargetValue = newTargetValue // Use propagated new.target
		newFrame.argCount = argCount             // Store actual argument count for arguments object
		// Copy arguments for arguments object (before registers get mutated by function execution)
		newFrame.args = make([]Value, argCount)
		copy(newFrame.args, spreadArgs)
		newFrame.argumentsObject = Undefined // Initialize to Undefined (will be created on first access)
		newFrame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]
		vm.nextRegSlot += requiredRegs

		// Copy spread arguments to new frame
		for i := 0; i < argCount && i < len(newFrame.registers); i++ {
			newFrame.registers[i] = spreadArgs[i]
		}
		vm.frameCount++

		// Store instance in caller's destination register
		callerRegisters[destReg] = newInstance

		// Return OK - caller will switch to new frame
		return InterpretOK, Undefined

	case TypeFunction:
		funcToCall := AsFunction(constructorVal)

		// Check if it's an arrow function
		if funcToCall.IsArrowFunction {
			frame.ip = callerIP
			vm.ThrowTypeError("Arrow functions cannot be used as constructors")
			return InterpretRuntimeError, Undefined
		}

		constructorClosure := &ClosureObject{Fn: funcToCall, Upvalues: []*Upvalue{}}
		constructorFunc := constructorClosure.Fn

		// Check stack limits
		if vm.frameCount == MaxFrames {
			frame.ip = callerIP
			status := vm.runtimeError("Stack overflow during constructor call.")
			return status, Undefined
		}
		requiredRegs := constructorFunc.RegisterSize
		if vm.nextRegSlot+requiredRegs > len(vm.registerStack) {
			frame.ip = callerIP
			status := vm.runtimeError("Register stack overflow during constructor call.")
			return status, Undefined
		}

		// Determine the new.target value for this constructor call
		// If the caller is already a constructor (super() call), inherit its new.target
		// Otherwise, new.target is the constructor being called
		var newTargetValue Value
		if frame.isConstructorCall && frame.newTargetValue.Type() != TypeUndefined {
			// This is a super() call from a derived constructor - inherit new.target
			newTargetValue = frame.newTargetValue
		} else {
			// Direct new Constructor() call - new.target is the constructor
			newTargetValue = constructorVal
		}

		// Get the prototype to use for the instance from new.target.prototype
		// This ensures derived classes create instances with the correct prototype
		var instancePrototype Value
		if newTargetValue.Type() == TypeClosure {
			newTargetClosure := AsClosure(newTargetValue)
			newTargetFunc := newTargetClosure.Fn
			instancePrototype = newTargetFunc.getOrCreatePrototypeWithVM(vm)
		} else if newTargetValue.Type() == TypeFunction {
			newTargetFunc := AsFunction(newTargetValue)
			instancePrototype = newTargetFunc.getOrCreatePrototypeWithVM(vm)
		} else {
			// Fallback: use the constructor's prototype
			instancePrototype = constructorFunc.getOrCreatePrototypeWithVM(vm)
		}

		// Create instance (or leave undefined for derived constructors)
		var newInstance Value
		if constructorFunc.IsDerivedConstructor {
			newInstance = Undefined
		} else {
			newInstance = NewObject(instancePrototype)
		}

		frame.ip = callerIP

		// Create new frame
		newFrame := &vm.frames[vm.frameCount]
		newFrame.closure = constructorClosure
		newFrame.ip = 0
		newFrame.targetRegister = destReg
		newFrame.thisValue = newInstance
		newFrame.homeObject = instancePrototype  // Set [[HomeObject]] for super property access in constructors
		newFrame.isConstructorCall = true
		newFrame.isDirectCall = false            // Not a direct call (spread new)
		newFrame.isSentinelFrame = false         // Clear sentinel flag when reusing frame
		newFrame.newTargetValue = newTargetValue // Use propagated new.target
		newFrame.argCount = argCount             // Store actual argument count for arguments object
		// Copy arguments for arguments object (before registers get mutated by function execution)
		newFrame.args = make([]Value, argCount)
		copy(newFrame.args, spreadArgs)
		newFrame.argumentsObject = Undefined // Initialize to Undefined (will be created on first access)
		newFrame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]
		vm.nextRegSlot += requiredRegs

		// Copy spread arguments to new frame
		for i := 0; i < argCount && i < len(newFrame.registers); i++ {
			newFrame.registers[i] = spreadArgs[i]
		}
		vm.frameCount++

		// Store instance in caller's destination register
		callerRegisters[destReg] = newInstance

		// Return OK - caller will switch to new frame
		return InterpretOK, Undefined

	case TypeNativeFunction, TypeNativeFunctionWithProps:
		nativeFunc := AsNativeFunction(constructorVal)

		// Native constructors handle their own instance creation
		result, nativeErr := nativeFunc.Fn(spreadArgs)
		if nativeErr != nil {
			frame.ip = callerIP
			status := vm.runtimeError("Native constructor error: %s", nativeErr.Error())
			return status, Undefined
		}

		callerRegisters[destReg] = result
		return InterpretOK, Undefined

	default:
		frame.ip = callerIP
		status := vm.runtimeError("Cannot use '%s' as a constructor.", constructorVal.TypeName())
		return status, Undefined
	}
}
