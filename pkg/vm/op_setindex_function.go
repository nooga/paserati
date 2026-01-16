package vm

// setFunctionProperty sets a property on a function object, handling accessors
func (vm *VM) setFunctionProperty(fnVal Value, key string, valueVal Value, ip int) (InterpretResult, Value) {
	fn := fnVal.AsFunction()

	// Function intrinsic properties "name" and "length" are non-writable per ECMAScript spec
	// Writes silently fail in non-strict mode
	if key == "name" || key == "length" {
		// TODO: In strict mode, this should throw TypeError
		// For now, silently fail (return success but don't modify)
		return InterpretOK, Undefined
	}

	// Check if this is an accessor property with a setter
	if fn.Properties != nil {
		if _, setter, _, _, ok := fn.Properties.GetOwnAccessor(key); ok && setter.Type() != TypeUndefined {
			// Call the setter with the value
			_, err := vm.Call(setter, fnVal, []Value{valueVal})
			if err != nil {
				if ee, ok := err.(ExceptionError); ok {
					vm.throwException(ee.GetExceptionValue())
					return InterpretRuntimeError, Undefined
				}
				status := vm.runtimeError("Error calling setter: %v", err)
				return status, Undefined
			}
			return InterpretOK, Undefined
		}
	}

	// No setter, set as data property
	if fn.Properties == nil {
		fn.Properties = NewObject(Undefined).AsPlainObject()
	}
	fn.Properties.SetOwn(key, valueVal)

	return InterpretOK, Undefined
}
