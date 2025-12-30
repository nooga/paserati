package vm

// handleOpDefineMethodComputed handles the OpDefineMethodComputed opcode
// This opcode defines a non-enumerable method on an object with a computed key
// Format: OpDefineMethodComputed ObjReg ValueReg KeyReg
func (vm *VM) handleOpDefineMethodComputed(code []byte, ip *int, registers []Value) (InterpretResult, Value) {
	objReg := code[*ip]
	valReg := code[*ip+1]
	keyReg := code[*ip+2]
	*ip += 3

	// Get the property key from register
	keyVal := registers[keyReg]

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

	// Create PropertyKey - handles both strings and symbols
	var propKey PropertyKey
	if keyVal.Type() == TypeSymbol {
		propKey = NewSymbolKey(keyVal)
	} else {
		propKey = NewStringKey(keyVal.ToString())
	}

	// Define as non-enumerable method using DefineOwnPropertyByKey
	writable := true
	enumerable := false  // Methods are non-enumerable per ECMAScript spec
	configurable := true

	if objVal.Type() == TypeObject {
		plainObj := objVal.AsPlainObject()
		plainObj.DefineOwnPropertyByKey(propKey, methodVal, &writable, &enumerable, &configurable)
		return InterpretOK, Undefined
	} else if objVal.Type() == TypeFunction {
		// Static class methods - add to the constructor function's properties
		// Per ECMAScript 14.5.14: Static method named "prototype" is forbidden
		if propKey.kind == KeyKindString && propKey.name == "prototype" {
			vm.ThrowTypeError("Classes may not have a static property named 'prototype'")
			return InterpretRuntimeError, Undefined
		}
		funcObj := objVal.AsFunction()
		if funcObj.Properties == nil {
			funcObj.Properties = &PlainObject{prototype: Undefined, shape: RootShape}
		}
		funcObj.Properties.DefineOwnPropertyByKey(propKey, methodVal, &writable, &enumerable, &configurable)
		return InterpretOK, Undefined
	} else if objVal.Type() == TypeClosure {
		// Static class methods on closures - add to the underlying function's properties
		// Per ECMAScript 14.5.14: Static method named "prototype" is forbidden
		if propKey.kind == KeyKindString && propKey.name == "prototype" {
			vm.ThrowTypeError("Classes may not have a static property named 'prototype'")
			return InterpretRuntimeError, Undefined
		}
		closure := objVal.AsClosure()
		if closure.Fn.Properties == nil {
			closure.Fn.Properties = &PlainObject{prototype: Undefined, shape: RootShape}
		}
		closure.Fn.Properties.DefineOwnPropertyByKey(propKey, methodVal, &writable, &enumerable, &configurable)
		return InterpretOK, Undefined
	} else {
		status := vm.runtimeError("Cannot define method on non-object type '%s'", objVal.TypeName())
		return status, Undefined
	}
}
