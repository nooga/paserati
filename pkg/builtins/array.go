package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"strings"
)

// registerArray registers the Array constructor and prototype methods
func registerArray() {
	// Array constructor is registered in registerArrayConstructor()
	// This function registers Array prototype methods (both runtime and types)
	registerArrayPrototypeMethods()
}

// registerArrayPrototypeMethods registers Array prototype methods with both implementations and types
func registerArrayPrototypeMethods() {
	// Register concat method
	vm.RegisterArrayPrototypeMethod("concat",
		vm.NewNativeFunction(-1, true, "concat", arrayPrototypeConcatImpl))
	RegisterPrototypeMethod("array", "concat", &types.FunctionType{
		ParameterTypes:    []types.Type{}, // No fixed parameters
		ReturnType:        &types.ArrayType{ElementType: types.Any},
		IsVariadic:        true,
		RestParameterType: &types.ArrayType{ElementType: types.Any}, // Accept any values
	})

	// Register push method
	vm.RegisterArrayPrototypeMethod("push",
		vm.NewNativeFunction(-1, true, "push", arrayPrototypePushImpl))
	RegisterPrototypeMethod("array", "push", &types.FunctionType{
		ParameterTypes:    []types.Type{}, // No fixed parameters
		ReturnType:        types.Number,   // Returns new length
		IsVariadic:        true,
		RestParameterType: &types.ArrayType{ElementType: types.Any}, // Accept any values
	})

	// Register pop method
	vm.RegisterArrayPrototypeMethod("pop",
		vm.NewNativeFunction(0, false, "pop", arrayPrototypePopImpl))
	RegisterPrototypeMethod("array", "pop", &types.FunctionType{
		ParameterTypes: []types.Type{},
		ReturnType:     types.Any,
		IsVariadic:     false,
	})

	// Register join method
	vm.RegisterArrayPrototypeMethod("join",
		vm.NewNativeFunction(1, false, "join", arrayPrototypeJoinImpl))
	RegisterPrototypeMethod("array", "join", &types.FunctionType{
		ParameterTypes: []types.Type{types.String}, // Optional separator parameter
		ReturnType:     types.String,               // Returns string
		IsVariadic:     false,
		OptionalParams: []bool{true}, // Separator is optional
	})
}

// arrayPrototypeConcatImpl implements Array.prototype.concat
func arrayPrototypeConcatImpl(args []vm.Value) vm.Value {
	// args[0] is 'this' (the array), args[1:] are values to concat
	if len(args) == 0 {
		return vm.NewArray()
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return vm.Undefined
	}

	// Create a new array with elements from this array
	newArray := vm.NewArray()
	newArrayObj := newArray.AsArray()

	// Copy all elements from the original array
	originalArray := thisArray.AsArray()
	for i := 0; i < originalArray.Length(); i++ {
		newArrayObj.Append(originalArray.Get(i))
	}

	// Append all additional arguments
	for i := 1; i < len(args); i++ {
		newArrayObj.Append(args[i])
	}

	return newArray
}

// arrayPrototypePushImpl implements Array.prototype.push
func arrayPrototypePushImpl(args []vm.Value) vm.Value {
	// args[0] is 'this' (the array), args[1:] are values to push
	if len(args) == 0 {
		return vm.Number(0)
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return vm.Undefined
	}

	arr := thisArray.AsArray()

	// Push all provided arguments to the array
	for i := 1; i < len(args); i++ {
		arr.Append(args[i])
	}

	// Return the new length
	return vm.Number(float64(arr.Length()))
}

// arrayPrototypePopImpl implements Array.prototype.pop
func arrayPrototypePopImpl(args []vm.Value) vm.Value {
	// args[0] is 'this' (the array)
	if len(args) == 0 {
		return vm.Undefined
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return vm.Undefined
	}

	arr := thisArray.AsArray()
	if arr.Length() == 0 {
		return vm.Undefined
	}

	// Get the last element
	lastElement := arr.Get(arr.Length() - 1)

	// Reduce the length by 1
	arr.SetLength(arr.Length() - 1)

	return lastElement
}

// arrayPrototypeJoinImpl implements Array.prototype.join
func arrayPrototypeJoinImpl(args []vm.Value) vm.Value {
	// args[0] is 'this' (the array), args[1] is optional separator
	if len(args) == 0 {
		return vm.NewString("")
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return vm.Undefined
	}

	// Get separator (default to comma)
	separator := ","
	if len(args) > 1 && args[1].Type() != vm.TypeUndefined {
		separator = args[1].ToString()
	}

	// Join array elements
	arr := thisArray.AsArray()
	if arr.Length() == 0 {
		return vm.NewString("")
	}

	var parts []string
	for i := 0; i < arr.Length(); i++ {
		element := arr.Get(i)
		parts = append(parts, element.ToString())
	}

	result := strings.Join(parts, separator)
	return vm.NewString(result)
}
