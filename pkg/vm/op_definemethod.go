package vm

// handleOpDefineMethod handles the OpDefineMethod opcode
// This opcode defines a non-enumerable method on an object (typically for class methods)
func (vm *VM) handleOpDefineMethod(code []byte, ip *int, constants []Value, registers []Value) (InterpretResult, Value) {
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
	enumerable := false // Methods are non-enumerable per ECMAScript spec
	configurable := true

	switch objVal.Type() {
	case TypeObject:
		plainObj := objVal.AsPlainObject()
		plainObj.DefineOwnProperty(methodName, methodVal, &writable, &enumerable, &configurable)
		return InterpretOK, Undefined

	case TypeFunction:
		fn := objVal.AsFunction()
		if fn.Properties == nil {
			fn.Properties = NewObject(Undefined).AsPlainObject()
		}
		fn.Properties.DefineOwnProperty(methodName, methodVal, &writable, &enumerable, &configurable)
		return InterpretOK, Undefined

	case TypeClosure:
		closure := objVal.AsClosure()
		if closure.Fn.Properties == nil {
			closure.Fn.Properties = NewObject(Undefined).AsPlainObject()
		}
		closure.Fn.Properties.DefineOwnProperty(methodName, methodVal, &writable, &enumerable, &configurable)
		return InterpretOK, Undefined

	case TypeNativeFunctionWithProps:
		nfp := objVal.AsNativeFunctionWithProps()
		if nfp.Properties == nil {
			nfp.Properties = NewObject(Undefined).AsPlainObject()
		}
		nfp.Properties.DefineOwnProperty(methodName, methodVal, &writable, &enumerable, &configurable)
		return InterpretOK, Undefined

	default:
		status := vm.runtimeError("Cannot define method on non-object type '%s'", objVal.TypeName())
		return status, Undefined
	}
}
