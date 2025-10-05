package vm

// handleOpSpreadNew handles OpSpreadNew bytecode instruction for constructor calls with spread arguments
func (vm *VM) handleOpSpreadNew(code []byte, ip *int, frame *CallFrame, registers []Value) (InterpretResult, Value) {
	destReg := code[*ip]
	constructorReg := code[*ip+1]
	spreadArgReg := code[*ip+2]
	*ip += 3

	callerRegisters := registers
	callerIP := *ip

	constructorVal := callerRegisters[constructorReg]
	spreadArrayVal := callerRegisters[spreadArgReg]

	// Extract arguments from spread array
	spreadArgs, err := vm.extractSpreadArguments(spreadArrayVal)
	if err != nil {
		frame.ip = callerIP
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

		// Get prototype
		instancePrototype := constructorFunc.getOrCreatePrototypeWithVM(vm)

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
		newFrame.isConstructorCall = true
		newFrame.newTargetValue = constructorVal
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

		// Get prototype
		instancePrototype := constructorFunc.getOrCreatePrototypeWithVM(vm)

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
		newFrame.isConstructorCall = true
		newFrame.newTargetValue = constructorVal
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
