package vm

// OpTypeGuardIterable checks if a value is iterable (has Symbol.iterator)
// If not iterable, throws TypeError
// Format: OpTypeGuardIterable srcReg
func (vm *VM) opTypeGuardIterable(srcReg int, registers []Value) (bool, InterpretResult, Value) {
	value := registers[srcReg]

	// Check if value has Symbol.iterator property
	// Null and undefined are not iterable
	if value.Type() == TypeNull || value.Type() == TypeUndefined {
		status := vm.runtimeError("%s is not iterable", value.TypeName())
		return false, status, Undefined
	}

	// For primitive types (string, number, boolean, symbol), check if they have Symbol.iterator
	// Strings are iterable, others are not
	switch value.Type() {
	case TypeString:
		// Strings are iterable
		return true, InterpretOK, Undefined
	case TypeFloatNumber, TypeIntegerNumber, TypeBoolean, TypeSymbol, TypeBigInt:
		// Not iterable
		status := vm.runtimeError("%s is not iterable", value.TypeName())
		return false, status, Undefined
	}

	// For objects, arrays, generators - they should have Symbol.iterator
	// Arrays and generators are always iterable
	switch value.Type() {
	case TypeArray, TypeTypedArray, TypeGenerator:
		// These are always iterable
		return true, InterpretOK, Undefined
	case TypeObject, TypeDictObject:
		// Need to check if Symbol.iterator exists
		// For now, optimistically assume objects with Symbol.iterator will work
		// The actual call to Symbol.iterator will fail if it doesn't exist
		return true, InterpretOK, Undefined
	}

	// Default: not iterable
	status := vm.runtimeError("%s is not iterable", value.TypeName())
	return false, status, Undefined
}

// OpTypeGuardIteratorReturn checks if iterator.return() result is an Object
// Per ECMAScript spec 7.4.6 IteratorClose step 9:
// If Type(innerResult.[[value]]) is not Object, throw a TypeError
// Format: OpTypeGuardIteratorReturn srcReg
func (vm *VM) opTypeGuardIteratorReturn(srcReg int, registers []Value) (bool, InterpretResult, Value) {
	value := registers[srcReg]

	// Check if value is an object type
	// According to ECMAScript, "Object" means any object (including arrays, functions, etc.)
	// but NOT primitives (null, undefined, number, string, boolean, symbol, bigint)
	switch value.Type() {
	case TypeObject, TypeDictObject, TypeArray, TypeTypedArray, TypeFunction, TypeRegExp:
		// These are all object types - valid
		return true, InterpretOK, Undefined
	default:
		// Primitives and null/undefined are NOT objects
		status := vm.runtimeError("Iterator result %s is not an object", value.TypeName())
		return false, status, Undefined
	}
}
