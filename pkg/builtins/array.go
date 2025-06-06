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

	// Register map method
	vm.RegisterArrayPrototypeMethod("map",
		vm.NewNativeFunction(1, false, "map", arrayPrototypeMapImpl))
	RegisterPrototypeMethod("array", "map", &types.FunctionType{
		ParameterTypes: []types.Type{&types.FunctionType{
			ParameterTypes: []types.Type{types.Any, types.Number, &types.ArrayType{ElementType: types.Any}},
			ReturnType:     types.Any,
			IsVariadic:     false,
		}},
		ReturnType: &types.ArrayType{ElementType: types.Any},
		IsVariadic: false,
	})

	// Register filter method
	vm.RegisterArrayPrototypeMethod("filter",
		vm.NewNativeFunction(1, false, "filter", arrayPrototypeFilterImpl))
	RegisterPrototypeMethod("array", "filter", &types.FunctionType{
		ParameterTypes: []types.Type{&types.FunctionType{
			ParameterTypes: []types.Type{types.Any, types.Number, &types.ArrayType{ElementType: types.Any}},
			ReturnType:     types.Boolean,
			IsVariadic:     false,
		}},
		ReturnType: &types.ArrayType{ElementType: types.Any},
		IsVariadic: false,
	})

	// Register forEach method
	vm.RegisterArrayPrototypeMethod("forEach",
		vm.NewNativeFunction(1, false, "forEach", arrayPrototypeForEachImpl))
	RegisterPrototypeMethod("array", "forEach", &types.FunctionType{
		ParameterTypes: []types.Type{&types.FunctionType{
			ParameterTypes: []types.Type{types.Any, types.Number, &types.ArrayType{ElementType: types.Any}},
			ReturnType:     types.Void,
			IsVariadic:     false,
		}},
		ReturnType: types.Void,
		IsVariadic: false,
	})

	// Register includes method
	vm.RegisterArrayPrototypeMethod("includes",
		vm.NewNativeFunction(1, false, "includes", arrayPrototypeIncludesImpl))
	RegisterPrototypeMethod("array", "includes", &types.FunctionType{
		ParameterTypes: []types.Type{types.Any},
		ReturnType:     types.Boolean,
		IsVariadic:     false,
	})

	// Register indexOf method
	vm.RegisterArrayPrototypeMethod("indexOf",
		vm.NewNativeFunction(1, false, "indexOf", arrayPrototypeIndexOfImpl))
	RegisterPrototypeMethod("array", "indexOf", &types.FunctionType{
		ParameterTypes: []types.Type{types.Any},
		ReturnType:     types.Number,
		IsVariadic:     false,
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

// arrayPrototypeMapImpl implements Array.prototype.map
func arrayPrototypeMapImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.NewArray()
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return vm.Undefined
	}

	callback := args[1]
	if !callback.IsNativeFunction() && !callback.IsFunction() && !callback.IsClosure() {
		return vm.Undefined
	}

	arr := thisArray.AsArray()
	newArray := vm.NewArray()
	newArrayObj := newArray.AsArray()

	for i := 0; i < arr.Length(); i++ {
		element := arr.Get(i)
		// Call callback with (element, index, array)
		callArgs := []vm.Value{element, vm.Number(float64(i)), thisArray}
		var result vm.Value
		if callback.IsNativeFunction() {
			nativeFn := callback.AsNativeFunction()
			result = nativeFn.Fn(callArgs)
		} else {
			// For compiled functions, we'd need VM support - skip for now
			result = vm.Undefined
		}
		newArrayObj.Append(result)
	}

	return newArray
}

// arrayPrototypeFilterImpl implements Array.prototype.filter
func arrayPrototypeFilterImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.NewArray()
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return vm.Undefined
	}

	callback := args[1]
	if !callback.IsNativeFunction() && !callback.IsFunction() && !callback.IsClosure() {
		return vm.Undefined
	}

	arr := thisArray.AsArray()
	newArray := vm.NewArray()
	newArrayObj := newArray.AsArray()

	for i := 0; i < arr.Length(); i++ {
		element := arr.Get(i)
		// Call callback with (element, index, array)
		callArgs := []vm.Value{element, vm.Number(float64(i)), thisArray}
		var result vm.Value
		if callback.IsNativeFunction() {
			nativeFn := callback.AsNativeFunction()
			result = nativeFn.Fn(callArgs)
		} else {
			// For compiled functions, we'd need VM support - skip for now
			result = vm.Undefined
		}
		if result.IsTruthy() {
			newArrayObj.Append(element)
		}
	}

	return newArray
}

// arrayPrototypeForEachImpl implements Array.prototype.forEach
func arrayPrototypeForEachImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.Undefined
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return vm.Undefined
	}

	callback := args[1]
	if !callback.IsNativeFunction() && !callback.IsFunction() && !callback.IsClosure() {
		return vm.Undefined
	}

	arr := thisArray.AsArray()

	for i := 0; i < arr.Length(); i++ {
		element := arr.Get(i)
		// Call callback with (element, index, array)
		callArgs := []vm.Value{element, vm.Number(float64(i)), thisArray}
		if callback.IsNativeFunction() {
			nativeFn := callback.AsNativeFunction()
			nativeFn.Fn(callArgs)
		} else {
			// For compiled functions, we'd need VM support - skip for now
		}
	}

	return vm.Undefined
}

// arrayPrototypeIncludesImpl implements Array.prototype.includes
func arrayPrototypeIncludesImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.BooleanValue(false)
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return vm.Undefined
	}

	searchElement := args[1]
	arr := thisArray.AsArray()

	for i := 0; i < arr.Length(); i++ {
		element := arr.Get(i)
		if element.Equals(searchElement) {
			return vm.BooleanValue(true)
		}
	}

	return vm.BooleanValue(false)
}

// arrayPrototypeIndexOfImpl implements Array.prototype.indexOf
func arrayPrototypeIndexOfImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.Number(-1)
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return vm.Undefined
	}

	searchElement := args[1]
	arr := thisArray.AsArray()

	for i := 0; i < arr.Length(); i++ {
		element := arr.Get(i)
		if element.Equals(searchElement) {
			return vm.Number(float64(i))
		}
	}

	return vm.Number(-1)
}
