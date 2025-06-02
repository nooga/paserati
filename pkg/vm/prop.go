package vm

import (
	"unicode/utf8"
)

// getProp handles property read for OpGetProp.
// Returns the property value or a runtime error status.
func (vm *VM) getProp(frame *CallFrame, objVal Value, propName string) (Value, InterpretResult) {
	// Initialize prototypes if needed
	initPrototypes()

	// Special case: length property on arrays/strings
	if propName == "length" {
		switch objVal.Type() {
		case TypeArray:
			arr := AsArray(objVal)
			return Number(float64(len(arr.elements))), InterpretOK
		case TypeString:
			str := AsString(objVal)
			return Number(float64(utf8.RuneCountInString(str))), InterpretOK
		}
	}

	// Handle String prototype methods
	if objVal.IsString() {
		if method, exists := StringPrototype[propName]; exists {
			return createBoundMethod(objVal, method), InterpretOK
		}
	}

	// Handle Array prototype methods
	if objVal.IsArray() {
		if method, exists := ArrayPrototype[propName]; exists {
			return createBoundMethod(objVal, method), InterpretOK
		}
	}

	// Must be object for regular property access
	if !objVal.IsObject() {
		if objVal.Type() == TypeNull || objVal.Type() == TypeUndefined {
			return Undefined, vm.runtimeError("Cannot read property '%s' of %s", propName, objVal.TypeName())
		}
		return Undefined, vm.runtimeError("Cannot access property '%s' on non-object type '%s'", propName, objVal.TypeName())
	}
	// Dispatch based on object subtype
	switch objVal.Type() {
	case TypeDictObject:
		dict := AsDictObject(objVal)
		if v, ok := dict.GetOwn(propName); ok {
			return v, InterpretOK
		}
		return Undefined, InterpretOK
	default:
		po := AsPlainObject(objVal)
		if v, ok := po.GetOwn(propName); ok {
			return v, InterpretOK
		}
		return Undefined, InterpretOK
	}
}

// setProp handles property write for OpSetProp.
// Returns InterpretOK or a runtime error status.
func (vm *VM) setProp(frame *CallFrame, objVal Value, propName string, valueToSet Value) InterpretResult {
	if !objVal.IsObject() {
		return vm.runtimeError("Cannot set property '%s' on non-object type '%s'", propName, objVal.TypeName())
	}
	switch objVal.Type() {
	case TypeDictObject:
		dict := AsDictObject(objVal)
		dict.SetOwn(propName, valueToSet)
	default:
		po := AsPlainObject(objVal)
		po.SetOwn(propName, valueToSet)
	}
	return InterpretOK
}
