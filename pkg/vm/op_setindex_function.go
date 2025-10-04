package vm

// setFunctionProperty sets a property on a function object, handling accessors
func (vm *VM) setFunctionProperty(fnVal Value, key string, valueVal Value, ip int) (InterpretResult, Value) {
	fn := fnVal.AsFunction()

	// Check if this is an accessor property with a setter
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
	} else {
		// No setter, set as data property
		fn.Properties.SetOwn(key, valueVal)
	}

	return InterpretOK, Undefined
}
