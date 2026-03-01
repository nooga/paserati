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

		// Object literal data properties are: writable=true, enumerable=true, configurable=true.
		//
		// Performance note:
		// - Most object literal properties are plain data properties (not accessors).
		// - For that common case, we want shape transitions (hidden-class reuse) so inline caches
		//   see a small, stable set of shapes instead of one shape per instance.
		// - We only need full DefineOwnProperty semantics when overwriting an accessor.

		// Fast path: if existing property is a data property (or missing), use SetOwn which uses
		// transitions and shares shapes across instances.
		for _, f := range plainObj.shape.fields {
			if f.keyKind == KeyKindString && f.name == propertyName {
				if !f.isAccessor {
					plainObj.SetOwn(propertyName, valueToSet)
					return InterpretOK, Undefined
				}
				// Accessor present: must redefine as a data property.
				writable := true
				enumerable := true
				configurable := true
				plainObj.DefineOwnProperty(propertyName, valueToSet, &writable, &enumerable, &configurable)
				return InterpretOK, Undefined
			}
		}

		// Missing property: SetOwn is correct (w/e/c all true) and uses transitions.
		plainObj.SetOwn(propertyName, valueToSet)
		return InterpretOK, Undefined
	} else {
		status := vm.runtimeError("Cannot define property on non-object type '%s'", objVal.TypeName())
		return status, Undefined
	}
}

// handleOpDefineComputedDataProperty handles the OpDefineComputedDataProperty opcode.
// Like OpDefineDataProperty but the property name comes from a register (computed key).
// Per ECMAScript, computed properties in object literals use CreateDataPropertyOrThrow,
// which is [[DefineOwnProperty]] — NOT [[Set]]. This is critical for __proto__:
// static `__proto__: v` sets prototype, but computed `['__proto__']: v` creates own property.
func (vm *VM) handleOpDefineComputedDataProperty(code []byte, ip *int, registers []Value) (InterpretResult, Value) {
	objReg := code[*ip]
	valReg := code[*ip+1]
	keyReg := code[*ip+2]
	*ip += 3

	objVal := registers[objReg]
	valueToSet := registers[valReg]
	keyVal := registers[keyReg]

	// Convert key to property name (string or symbol)
	propertyName := keyVal.ToString()

	if objVal.Type() == TypeObject {
		plainObj := objVal.AsPlainObject()

		// Check for symbol keys
		if keyVal.IsSymbol() {
			symKey := NewSymbolKey(keyVal)
			wTrue, eTrue, cTrue := true, true, true
			plainObj.DefineOwnPropertyByKey(symKey, valueToSet, &wTrue, &eTrue, &cTrue)
			return InterpretOK, Undefined
		}

		// Fast path: if existing property is a data property, use SetOwn for shape reuse
		for _, f := range plainObj.shape.fields {
			if f.keyKind == KeyKindString && f.name == propertyName {
				if !f.isAccessor {
					plainObj.SetOwn(propertyName, valueToSet)
					return InterpretOK, Undefined
				}
				// Accessor present: must redefine as a data property
				writable := true
				enumerable := true
				configurable := true
				plainObj.DefineOwnProperty(propertyName, valueToSet, &writable, &enumerable, &configurable)
				return InterpretOK, Undefined
			}
		}

		// Missing property: SetOwn creates w/e/c all true
		plainObj.SetOwn(propertyName, valueToSet)
		return InterpretOK, Undefined
	}

	status := vm.runtimeError("Cannot define property on non-object type '%s'", objVal.TypeName())
	return status, Undefined
}
