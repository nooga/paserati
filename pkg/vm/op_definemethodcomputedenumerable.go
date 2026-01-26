package vm

// handleOpDefineMethodComputedEnumerable handles the OpDefineMethodComputedEnumerable opcode
// This opcode defines an enumerable method on an object with a computed key (for object literal methods)
// Format: OpDefineMethodComputedEnumerable ObjReg ValueReg KeyReg
func (vm *VM) handleOpDefineMethodComputedEnumerable(code []byte, ip *int, registers []Value) (InterpretResult, Value) {
	objReg := code[*ip]
	valReg := code[*ip+1]
	keyReg := code[*ip+2]
	*ip += 3

	// Get the property key from register
	keyVal := registers[keyReg]

	// Define method as enumerable (for object literals)
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

	// Create PropertyKey - handles both strings and symbols
	var propKey PropertyKey
	if keyVal.Type() == TypeSymbol {
		propKey = NewSymbolKey(keyVal)
	} else {
		// For objects/callables, use toPrimitive to get proper toString() call
		var keyStr string
		if keyVal.IsObject() || keyVal.IsCallable() {
			primitiveVal := vm.toPrimitive(keyVal, "string")
			if vm.unwinding {
				return InterpretRuntimeError, Undefined
			}
			keyStr = primitiveVal.ToString()
		} else {
			keyStr = keyVal.ToString()
		}
		propKey = NewStringKey(keyStr)
	}

	// SetFunctionName: update the method's .name based on the computed key
	setComputedFunctionName(methodVal, propKey, "", true)

	// Define as enumerable method (object literal methods are enumerable per ECMAScript spec)
	writable := true
	enumerable := true // Object literal methods are enumerable
	configurable := true

	if objVal.Type() == TypeObject {
		plainObj := objVal.AsPlainObject()
		plainObj.DefineOwnPropertyByKey(propKey, methodVal, &writable, &enumerable, &configurable)
		return InterpretOK, Undefined
	} else {
		status := vm.runtimeError("Cannot define method on non-object type '%s'", objVal.TypeName())
		return status, Undefined
	}
}
