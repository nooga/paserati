package vm

// setComputedFunctionName implements the ECMAScript SetFunctionName abstract operation
// for computed property keys. It updates the function's .name property based on the key.
// prefix is prepended (e.g., "get " or "set " for accessors).
// If onlyIfAnonymous is true, the name is only set when the function has no existing name
// (for property assignments where named function expressions keep their name).
func setComputedFunctionName(methodVal Value, propKey PropertyKey, prefix string, onlyIfAnonymous bool) {
	var name string
	if propKey.kind == KeyKindSymbol {
		// Per spec: if key is a Symbol with description, name is "[description]"
		// If description is undefined (empty in our representation), name is ""
		desc := propKey.symbolVal.AsSymbol()
		if desc != "" {
			name = "[" + desc + "]"
		}
	} else {
		name = propKey.name
	}
	if prefix != "" {
		name = prefix + name
	}
	if methodVal.Type() == TypeClosure {
		fn := methodVal.AsClosure().Fn
		if !onlyIfAnonymous || fn.Name == "" {
			fn.Name = name
		}
	} else if methodVal.Type() == TypeFunction {
		fn := AsFunction(methodVal)
		if !onlyIfAnonymous || fn.Name == "" {
			fn.Name = name
		}
	}
}

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
	// Per ECMAScript spec, if key is a symbol, name is "[description]"
	// If key is a string, name is that string
	setComputedFunctionName(methodVal, propKey, "", false)

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
		// Static class methods on closures - add to closure's own Properties for per-closure isolation
		// Per ECMAScript 14.5.14: Static method named "prototype" is forbidden
		if propKey.kind == KeyKindString && propKey.name == "prototype" {
			vm.ThrowTypeError("Classes may not have a static property named 'prototype'")
			return InterpretRuntimeError, Undefined
		}
		closure := objVal.AsClosure()
		// Use closure's own Properties for per-closure isolation (consistent with OpSetProp)
		if closure.Properties == nil {
			closure.Properties = &PlainObject{prototype: Undefined, shape: RootShape}
		}
		closure.Properties.DefineOwnPropertyByKey(propKey, methodVal, &writable, &enumerable, &configurable)
		return InterpretOK, Undefined
	} else {
		status := vm.runtimeError("Cannot define method on non-object type '%s'", objVal.TypeName())
		return status, Undefined
	}
}
