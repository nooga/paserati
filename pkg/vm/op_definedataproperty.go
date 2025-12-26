package vm

// handleOpDefineDataProperty handles the OpDefineDataProperty opcode
// This opcode defines an enumerable data property on an object using DefineOwnProperty semantics.
// This is used for object literal data properties and can overwrite existing properties
// including accessors.
func (vm *VM) handleOpDefineDataProperty(code []byte, ip *int, constants []Value, registers []Value) (InterpretResult, Value) {
	objReg := code[*ip]
	valReg := code[*ip+1]
	nameConstIdxHi := code[*ip+2]
	nameConstIdxLo := code[*ip+3]
	nameConstIdx := uint16(nameConstIdxHi)<<8 | uint16(nameConstIdxLo)
	*ip += 4

	// Get property name from constants
	if int(nameConstIdx) >= len(constants) {
		status := vm.runtimeError("Invalid constant index %d for property name.", nameConstIdx)
		return status, Undefined
	}
	nameVal := constants[nameConstIdx]
	if !IsString(nameVal) {
		status := vm.runtimeError("Internal Error: Property name constant %d is not a string.", nameConstIdx)
		return status, Undefined
	}
	propertyName := AsString(nameVal)

	// Get object and value
	objVal := registers[objReg]
	valueToSet := registers[valReg]

	if objVal.Type() == TypeObject {
		plainObj := objVal.AsPlainObject()

		// Use DefineOwnProperty which can overwrite any existing property (including accessors)
		// Object literal data properties are: writable=true, enumerable=true, configurable=true
		writable := true
		enumerable := true
		configurable := true
		plainObj.DefineOwnProperty(propertyName, valueToSet, &writable, &enumerable, &configurable)
		return InterpretOK, Undefined
	} else {
		status := vm.runtimeError("Cannot define property on non-object type '%s'", objVal.TypeName())
		return status, Undefined
	}
}
