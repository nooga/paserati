package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// registerObjectConstructor registers the Object constructor and prototype methods
func registerObjectConstructor() {
	// Create Object type with static methods using ObjectType pattern
	objectType := types.NewObjectType().
		WithCallSignature(types.Sig([]types.Type{types.Any}, types.Any).
			WithOptional(true)).
		WithConstructSignature(types.Sig([]types.Type{types.Any}, types.Any).
			WithOptional(true)).
		WithProperty("getPrototypeOf", types.NewSimpleFunction([]types.Type{types.Any}, types.Any))
	
	// Create the runtime Object function with properties
	objectValue := vm.NewNativeFunctionWithProps(0, true, "Object", objectConstructor)
	objectObj := objectValue.AsNativeFunctionWithProps()
	
	// Add the getPrototypeOf method to the runtime Object
	objectObj.Properties.SetOwn("getPrototypeOf", vm.NewNativeFunction(1, false, "getPrototypeOf", objectGetPrototypeOf))
	
	// Register using registerObject since it's a function with properties
	registerObject("Object", objectValue, objectType)
}

// objectConstructor implements the Object constructor
func objectConstructor(args []vm.Value) vm.Value {
	if len(args) == 0 {
		// new Object() or Object() returns a new empty object
		obj := vm.NewObject(vm.DefaultObjectPrototype)
		return obj
	}
	
	// Convert the argument to an object
	arg := args[0]
	switch arg.Type() {
	case vm.TypeObject:
		// Already an object, return as-is
		return arg
	case vm.TypeNull, vm.TypeUndefined:
		// null or undefined -> new empty object
		obj := vm.NewObject(vm.DefaultObjectPrototype)
		return obj
	default:
		// Primitive values get boxed (not fully implemented yet)
		// For now, just return a new object
		obj := vm.NewObject(vm.DefaultObjectPrototype)
		return obj
	}
}

// objectGetPrototypeOf implements Object.getPrototypeOf() static method
func objectGetPrototypeOf(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Undefined
	}
	
	obj := args[0]
	
	// For objects with prototypes, return their prototype
	switch obj.Type() {
	case vm.TypeObject:
		// For plain objects, get their actual prototype
		plainObj := obj.AsPlainObject()
		if plainObj != nil {
			return plainObj.GetPrototype()
		}
		return vm.Null
	case vm.TypeArray:
		// For arrays, return Array.prototype if available
		if vm.ArrayPrototype != nil {
			return vm.NewValueFromPlainObject(vm.ArrayPrototype)
		}
		return vm.DefaultObjectPrototype
	case vm.TypeString:
		// For strings, return String.prototype if available
		if vm.StringPrototype != nil {
			return vm.NewValueFromPlainObject(vm.StringPrototype)
		}
		return vm.Null
	case vm.TypeFunction, vm.TypeClosure:
		// For functions, return Function.prototype if available
		if vm.FunctionPrototype != nil {
			return vm.NewValueFromPlainObject(vm.FunctionPrototype)
		}
		return vm.Null
	default:
		// For primitive values, return null
		return vm.Null
	}
}