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
	RegisterPrototypeMethod("array", "concat",
		types.NewVariadicFunction([]types.Type{}, &types.ArrayType{ElementType: types.Any}, &types.ArrayType{ElementType: types.Any}))

	// Register push method
	vm.RegisterArrayPrototypeMethod("push",
		vm.NewNativeFunction(-1, true, "push", arrayPrototypePushImpl))
	RegisterPrototypeMethod("array", "push",
		types.NewVariadicFunction([]types.Type{}, types.Number, &types.ArrayType{ElementType: types.Any}))

	// Register pop method
	vm.RegisterArrayPrototypeMethod("pop",
		vm.NewNativeFunction(0, false, "pop", arrayPrototypePopImpl))
	RegisterPrototypeMethod("array", "pop",
		types.NewSimpleFunction([]types.Type{}, types.Any))

	// Register join method
	vm.RegisterArrayPrototypeMethod("join",
		vm.NewNativeFunction(1, false, "join", arrayPrototypeJoinImpl))
	RegisterPrototypeMethod("array", "join",
		types.NewSignature(types.String).
			WithOptional(true).
			Returns(types.String).
			ToFunction())

	// Register map method
	vm.RegisterArrayPrototypeMethod("map",
		vm.NewNativeFunction(1, false, "map", arrayPrototypeMapImpl))
	callbackType := types.NewSimpleFunction([]types.Type{types.Any}, types.Any)
	RegisterPrototypeMethod("array", "map",
		types.NewSimpleFunction([]types.Type{callbackType}, &types.ArrayType{ElementType: types.Any}))

	// Register filter method
	vm.RegisterArrayPrototypeMethod("filter",
		vm.NewNativeFunction(1, false, "filter", arrayPrototypeFilterImpl))
	filterCallbackType := types.NewSimpleFunction([]types.Type{types.Any}, types.Any)
	RegisterPrototypeMethod("array", "filter",
		types.NewSimpleFunction([]types.Type{filterCallbackType}, &types.ArrayType{ElementType: types.Any}))

	// Register forEach method
	vm.RegisterArrayPrototypeMethod("forEach",
		vm.NewNativeFunction(1, false, "forEach", arrayPrototypeForEachImpl))
	forEachCallbackType := types.NewSimpleFunction([]types.Type{types.Any}, types.Any)
	RegisterPrototypeMethod("array", "forEach",
		types.NewSimpleFunction([]types.Type{forEachCallbackType}, types.Void))

	// Register includes method
	vm.RegisterArrayPrototypeMethod("includes",
		vm.NewNativeFunction(1, false, "includes", arrayPrototypeIncludesImpl))
	RegisterPrototypeMethod("array", "includes",
		types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean))

	// Register indexOf method
	vm.RegisterArrayPrototypeMethod("indexOf",
		vm.NewNativeFunction(1, false, "indexOf", arrayPrototypeIndexOfImpl))
	RegisterPrototypeMethod("array", "indexOf",
		types.NewSimpleFunction([]types.Type{types.Any}, types.Number))

	// Register reverse method
	vm.RegisterArrayPrototypeMethod("reverse",
		vm.NewNativeFunction(0, false, "reverse", arrayPrototypeReverseImpl))
	RegisterPrototypeMethod("array", "reverse",
		types.NewSimpleFunction([]types.Type{}, &types.ArrayType{ElementType: types.Any}))

	// Register slice method
	vm.RegisterArrayPrototypeMethod("slice",
		vm.NewNativeFunction(2, false, "slice", arrayPrototypeSliceImpl))
	RegisterPrototypeMethod("array", "slice",
		types.NewSignature(types.Number, types.Number).
			WithOptional(true, true).
			Returns(&types.ArrayType{ElementType: types.Any}).
			ToFunction())

	// Register lastIndexOf method
	vm.RegisterArrayPrototypeMethod("lastIndexOf",
		vm.NewNativeFunction(1, false, "lastIndexOf", arrayPrototypeLastIndexOfImpl))
	RegisterPrototypeMethod("array", "lastIndexOf",
		types.NewSignature(types.Any).
			Returns(types.Number).
			ToFunction())

	// Register shift method
	vm.RegisterArrayPrototypeMethod("shift",
		vm.NewNativeFunction(0, false, "shift", arrayPrototypeShiftImpl))
	RegisterPrototypeMethod("array", "shift",
		types.NewSignature().
			Returns(types.Any).
			ToFunction())

	// Register unshift method
	vm.RegisterArrayPrototypeMethod("unshift",
		vm.NewNativeFunction(-1, true, "unshift", arrayPrototypeUnshiftImpl))
	RegisterPrototypeMethod("array", "unshift",
		types.NewSignature().
			WithRest(types.Any).
			Returns(types.Number).
			ToFunction())

	// Register toString method
	vm.RegisterArrayPrototypeMethod("toString",
		vm.NewNativeFunction(0, false, "toString", arrayPrototypeToStringImpl))
	RegisterPrototypeMethod("array", "toString",
		types.NewSignature().
			Returns(types.String).
			ToFunction())

	// Register every method
	vm.RegisterArrayPrototypeMethod("every",
		vm.NewNativeFunction(1, false, "every", arrayPrototypeEveryImpl))
	everyCallbackType := types.NewSignature(types.Any).Returns(types.Any).ToFunction()
	RegisterPrototypeMethod("array", "every",
		types.NewSignature(everyCallbackType).
			Returns(types.Boolean).
			ToFunction())

	// Register some method
	vm.RegisterArrayPrototypeMethod("some",
		vm.NewNativeFunction(1, false, "some", arrayPrototypeSomeImpl))
	someCallbackType := types.NewSignature(types.Any).Returns(types.Any).ToFunction()
	RegisterPrototypeMethod("array", "some",
		types.NewSignature(someCallbackType).
			Returns(types.Boolean).
			ToFunction())

	// Register find method
	vm.RegisterArrayPrototypeMethod("find",
		vm.NewNativeFunction(1, false, "find", arrayPrototypeFindImpl))
	findCallbackType := types.NewSignature(types.Any).Returns(types.Any).ToFunction()
	RegisterPrototypeMethod("array", "find",
		types.NewSignature(findCallbackType).
			Returns(types.Any).
			ToFunction())

	// Register findIndex method
	vm.RegisterArrayPrototypeMethod("findIndex",
		vm.NewNativeFunction(1, false, "findIndex", arrayPrototypeFindIndexImpl))
	findIndexCallbackType := types.NewSignature(types.Any).Returns(types.Any).ToFunction()
	RegisterPrototypeMethod("array", "findIndex",
		types.NewSignature(findIndexCallbackType).
			Returns(types.Number).
			ToFunction())
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

// arrayPrototypeReverseImpl implements Array.prototype.reverse
func arrayPrototypeReverseImpl(args []vm.Value) vm.Value {
	if len(args) < 1 {
		return vm.Undefined
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return vm.Undefined
	}

	arr := thisArray.AsArray()
	length := arr.Length()

	// Reverse elements in place
	for i := 0; i < length/2; i++ {
		j := length - 1 - i
		temp := arr.Get(i)
		arr.Set(i, arr.Get(j))
		arr.Set(j, temp)
	}

	return thisArray // Return the same array (mutated)
}

// arrayPrototypeSliceImpl implements Array.prototype.slice
func arrayPrototypeSliceImpl(args []vm.Value) vm.Value {
	if len(args) < 1 {
		return vm.NewArray()
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return vm.Undefined
	}

	arr := thisArray.AsArray()
	length := arr.Length()

	// Default start to 0
	start := 0
	if len(args) > 1 && args[1].Type() != vm.TypeUndefined {
		start = int(args[1].ToFloat())
	}

	// Default end to length
	end := length
	if len(args) > 2 && args[2].Type() != vm.TypeUndefined {
		end = int(args[2].ToFloat())
	}

	// Handle negative indices
	if start < 0 {
		start = length + start
		if start < 0 {
			start = 0
		}
	}
	if end < 0 {
		end = length + end
		if end < 0 {
			end = 0
		}
	}

	// Clamp to array bounds
	if start > length {
		start = length
	}
	if end > length {
		end = length
	}

	// Create new array with sliced elements
	newArray := vm.NewArray()
	newArrayObj := newArray.AsArray()

	for i := start; i < end; i++ {
		newArrayObj.Append(arr.Get(i))
	}

	return newArray
}

// arrayPrototypeLastIndexOfImpl implements Array.prototype.lastIndexOf
func arrayPrototypeLastIndexOfImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.Number(-1)
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return vm.Undefined
	}

	searchElement := args[1]
	arr := thisArray.AsArray()

	// Search from end to beginning
	for i := arr.Length() - 1; i >= 0; i-- {
		element := arr.Get(i)
		if element.Equals(searchElement) {
			return vm.Number(float64(i))
		}
	}

	return vm.Number(-1)
}

// arrayPrototypeShiftImpl implements Array.prototype.shift
func arrayPrototypeShiftImpl(args []vm.Value) vm.Value {
	if len(args) < 1 {
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

	// Get first element
	firstElement := arr.Get(0)

	// Shift all elements left
	for i := 1; i < arr.Length(); i++ {
		arr.Set(i-1, arr.Get(i))
	}

	// Reduce length by 1
	arr.SetLength(arr.Length() - 1)

	return firstElement
}

// arrayPrototypeUnshiftImpl implements Array.prototype.unshift
func arrayPrototypeUnshiftImpl(args []vm.Value) vm.Value {
	if len(args) < 1 {
		return vm.Number(0)
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return vm.Undefined
	}

	arr := thisArray.AsArray()
	elementsToAdd := len(args) - 1

	if elementsToAdd == 0 {
		return vm.Number(float64(arr.Length()))
	}

	oldLength := arr.Length()
	newLength := oldLength + elementsToAdd

	// First, extend the array
	for i := 0; i < elementsToAdd; i++ {
		arr.Append(vm.Undefined) // Add placeholders
	}

	// Shift existing elements to the right
	for i := oldLength - 1; i >= 0; i-- {
		arr.Set(i+elementsToAdd, arr.Get(i))
	}

	// Insert new elements at the beginning
	for i := 0; i < elementsToAdd; i++ {
		arr.Set(i, args[i+1])
	}

	return vm.Number(float64(newLength))
}

// arrayPrototypeToStringImpl implements Array.prototype.toString
func arrayPrototypeToStringImpl(args []vm.Value) vm.Value {
	if len(args) < 1 {
		return vm.NewString("")
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return vm.Undefined
	}

	// toString() is equivalent to join(",")
	return arrayPrototypeJoinImpl([]vm.Value{thisArray, vm.NewString(",")})
}

// arrayPrototypeEveryImpl implements Array.prototype.every
func arrayPrototypeEveryImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.BooleanValue(true) // Empty array returns true
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
		var result vm.Value
		if callback.IsNativeFunction() {
			nativeFn := callback.AsNativeFunction()
			result = nativeFn.Fn(callArgs)
		} else {
			// For compiled functions, we'd need VM support - skip for now
			result = vm.BooleanValue(true) // Default to true for now
		}
		if !result.IsTruthy() {
			return vm.BooleanValue(false)
		}
	}

	return vm.BooleanValue(true)
}

// arrayPrototypeSomeImpl implements Array.prototype.some
func arrayPrototypeSomeImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.BooleanValue(false) // Empty array returns false
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
		var result vm.Value
		if callback.IsNativeFunction() {
			nativeFn := callback.AsNativeFunction()
			result = nativeFn.Fn(callArgs)
		} else {
			// For compiled functions, we'd need VM support - skip for now
			result = vm.BooleanValue(false) // Default to false for now
		}
		if result.IsTruthy() {
			return vm.BooleanValue(true)
		}
	}

	return vm.BooleanValue(false)
}

// arrayPrototypeFindImpl implements Array.prototype.find
func arrayPrototypeFindImpl(args []vm.Value) vm.Value {
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
		var result vm.Value
		if callback.IsNativeFunction() {
			nativeFn := callback.AsNativeFunction()
			result = nativeFn.Fn(callArgs)
		} else {
			// For compiled functions, we'd need VM support - skip for now
			result = vm.BooleanValue(false) // Default to false for now
		}
		if result.IsTruthy() {
			return element
		}
	}

	return vm.Undefined
}

// arrayPrototypeFindIndexImpl implements Array.prototype.findIndex
func arrayPrototypeFindIndexImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.Number(-1)
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
		var result vm.Value
		if callback.IsNativeFunction() {
			nativeFn := callback.AsNativeFunction()
			result = nativeFn.Fn(callArgs)
		} else {
			// For compiled functions, we'd need VM support - skip for now
			result = vm.BooleanValue(false) // Default to false for now
		}
		if result.IsTruthy() {
			return vm.Number(float64(i))
		}
	}

	return vm.Number(-1)
}
