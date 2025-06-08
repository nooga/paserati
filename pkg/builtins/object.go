package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// registerObjectConstructor registers the Object constructor and prototype methods
func registerObjectConstructor() {
	// Object constructor - can be called with new or without
	// In TypeScript, Object(value) converts the value to an object
	// new Object(value) creates a new object wrapper
	objectType := types.NewFunctionType(&types.Signature{
		ParameterTypes: []types.Type{types.Any},
		ReturnType:     types.Any, // Returns any type when called with new
		OptionalParams: []bool{true}, // Parameter is optional
	})
	
	// Also add a construct signature to make instanceof work
	objectType.ConstructSignatures = append(objectType.ConstructSignatures, &types.Signature{
		ParameterTypes: []types.Type{types.Any},
		ReturnType:     types.Any,
		OptionalParams: []bool{true},
	})
	
	// Register the Object constructor
	register("Object", 0, true, objectConstructor, objectType)
	
	// TODO: Add Object static methods like Object.create, Object.keys, etc.
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