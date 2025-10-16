package vm

// handleOpDefineMethodEnumerable handles the OpDefineMethodEnumerable opcode
// This opcode defines an enumerable method on an object (typically for object literal methods)
func (vm *VM) handleOpDefineMethodEnumerable(code []byte, ip *int, constants []Value, registers []Value) (InterpretResult, Value) {
	objReg := code[*ip]
	valReg := code[*ip+1]
	nameConstIdxHi := code[*ip+2]
	nameConstIdxLo := code[*ip+3]
	nameConstIdx := uint16(nameConstIdxHi)<<8 | uint16(nameConstIdxLo)
	*ip += 4

	// Get property name from constants
	if int(nameConstIdx) >= len(constants) {
		status := vm.runtimeError("Invalid constant index %d for method name.", nameConstIdx)
		return status, Undefined
	}
	nameVal := constants[nameConstIdx]
	if !IsString(nameVal) {
		status := vm.runtimeError("Internal Error: Method name constant %d is not a string.", nameConstIdx)
		return status, Undefined
	}
	methodName := AsString(nameVal)

	// Define method as non-enumerable
	objVal := registers[objReg]
	methodVal := registers[valReg]

	if objVal.Type() == TypeObject {
		plainObj := objVal.AsPlainObject()

		// Set [[HomeObject]] on the method closure for super property access
		// Per ECMAScript spec, methods defined with method syntax get a [[HomeObject]]
		// pointing to the object where the method is defined
		if methodVal.Type() == TypeClosure {
			closure := methodVal.AsClosure()
			closure.Fn.HomeObject = objVal
		} else if methodVal.Type() == TypeFunction {
			// Bare FunctionObject (not yet wrapped in closure)
			funcObj := AsFunction(methodVal)
			funcObj.HomeObject = objVal
		}

		writable := true
		enumerable := true  // Object literal methods are enumerable
		configurable := true
		plainObj.DefineOwnProperty(methodName, methodVal, &writable, &enumerable, &configurable)
		return InterpretOK, Undefined
	} else {
		status := vm.runtimeError("Cannot define method on non-object type '%s'", objVal.TypeName())
		return status, Undefined
	}
}
