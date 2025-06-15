package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

type ArrayInitializer struct{}

func (a *ArrayInitializer) Name() string {
	return "Array"
}

func (a *ArrayInitializer) Priority() int {
	return PriorityArray // 2 - After Object (0) and Function (1)
}

func (a *ArrayInitializer) InitTypes(ctx *TypeContext) error {
	// Create Array.prototype type with all methods
	arrayProtoType := types.NewObjectType().
		WithProperty("length", types.Number).
		WithVariadicProperty("push", []types.Type{}, types.Number, &types.ArrayType{ElementType: types.Any}).
		WithProperty("pop", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("shift", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithVariadicProperty("unshift", []types.Type{}, types.Number, &types.ArrayType{ElementType: types.Any}).
		WithProperty("slice", types.NewSimpleFunction([]types.Type{types.Number, types.Number}, &types.ArrayType{ElementType: types.Any})).
		WithVariadicProperty("splice", []types.Type{types.Number, types.Number}, &types.ArrayType{ElementType: types.Any}, &types.ArrayType{ElementType: types.Any}).
		WithVariadicProperty("concat", []types.Type{}, &types.ArrayType{ElementType: types.Any}, &types.ArrayType{ElementType: types.Any}).
		WithProperty("join", types.NewSimpleFunction([]types.Type{types.String}, types.String)).
		WithProperty("reverse", types.NewSimpleFunction([]types.Type{}, &types.ArrayType{ElementType: types.Any})).
		WithProperty("sort", types.NewSimpleFunction([]types.Type{types.NewSimpleFunction([]types.Type{types.Any, types.Any}, types.Number)}, &types.ArrayType{ElementType: types.Any})).
		WithProperty("indexOf", types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Number)).
		WithProperty("lastIndexOf", types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Number)).
		WithProperty("includes", types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Boolean)).
		WithProperty("find", types.NewSimpleFunction([]types.Type{types.NewSimpleFunction([]types.Type{types.Any, types.Number, &types.ArrayType{ElementType: types.Any}}, types.Boolean)}, types.Any)).
		WithProperty("findIndex", types.NewSimpleFunction([]types.Type{types.NewSimpleFunction([]types.Type{types.Any, types.Number, &types.ArrayType{ElementType: types.Any}}, types.Boolean)}, types.Number)).
		WithProperty("filter", types.NewSimpleFunction([]types.Type{types.NewSimpleFunction([]types.Type{types.Any, types.Number, &types.ArrayType{ElementType: types.Any}}, types.Boolean)}, &types.ArrayType{ElementType: types.Any})).
		WithProperty("map", types.NewSimpleFunction([]types.Type{types.NewSimpleFunction([]types.Type{types.Any, types.Number, &types.ArrayType{ElementType: types.Any}}, types.Any)}, &types.ArrayType{ElementType: types.Any})).
		WithProperty("forEach", types.NewSimpleFunction([]types.Type{types.NewSimpleFunction([]types.Type{types.Any, types.Number, &types.ArrayType{ElementType: types.Any}}, types.Undefined)}, types.Undefined)).
		WithProperty("every", types.NewSimpleFunction([]types.Type{types.NewSimpleFunction([]types.Type{types.Any, types.Number, &types.ArrayType{ElementType: types.Any}}, types.Boolean)}, types.Boolean)).
		WithProperty("some", types.NewSimpleFunction([]types.Type{types.NewSimpleFunction([]types.Type{types.Any, types.Number, &types.ArrayType{ElementType: types.Any}}, types.Boolean)}, types.Boolean)).
		WithProperty("reduce", types.NewSimpleFunction([]types.Type{types.NewSimpleFunction([]types.Type{types.Any, types.Any, types.Number, &types.ArrayType{ElementType: types.Any}}, types.Any), types.Any}, types.Any)).
		WithProperty("reduceRight", types.NewSimpleFunction([]types.Type{types.NewSimpleFunction([]types.Type{types.Any, types.Any, types.Number, &types.ArrayType{ElementType: types.Any}}, types.Any), types.Any}, types.Any))

	// Register array primitive prototype
	ctx.SetPrimitivePrototype("array", arrayProtoType)

	// Create Array constructor type
	arrayCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, &types.ArrayType{ElementType: types.Any}).                                             // Array() -> array
		WithSimpleCallSignature([]types.Type{types.Number}, &types.ArrayType{ElementType: types.Any}).                                 // Array(length) -> array
		WithVariadicCallSignature([]types.Type{}, &types.ArrayType{ElementType: types.Any}, &types.ArrayType{ElementType: types.Any}). // Array(...elements) -> array
		WithProperty("isArray", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("from", types.NewOptionalFunction([]types.Type{types.Any, types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Any)}, &types.ArrayType{ElementType: types.Any}, []bool{false, true})).
		WithVariadicProperty("of", []types.Type{}, &types.ArrayType{ElementType: types.Any}, &types.ArrayType{ElementType: types.Any}).
		WithProperty("prototype", arrayProtoType)

	// Define Array constructor in global environment
	return ctx.DefineGlobal("Array", arrayCtorType)
}

func (a *ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create Array.prototype inheriting from Object.prototype
	arrayProto := vm.NewObject(objectProto).AsPlainObject()

	// Add Array prototype methods
	arrayProto.SetOwn("push", vm.NewNativeFunction(0, true, "push", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.NumberValue(0)
		}
		for i := 0; i < len(args); i++ {
			thisArray.Append(args[i])
		}
		return vm.NumberValue(float64(thisArray.Length()))
	}))

	arrayProto.SetOwn("pop", vm.NewNativeFunction(0, false, "pop", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || thisArray.Length() == 0 {
			return vm.Undefined
		}
		lastIndex := thisArray.Length() - 1
		lastElement := thisArray.Get(lastIndex)
		thisArray.SetLength(lastIndex)
		return lastElement
	}))

	arrayProto.SetOwn("shift", vm.NewNativeFunction(0, false, "shift", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || thisArray.Length() == 0 {
			return vm.Undefined
		}
		firstElement := thisArray.Get(0)
		// Shift all elements left
		newElements := make([]vm.Value, thisArray.Length()-1)
		for i := 1; i < thisArray.Length(); i++ {
			newElements[i-1] = thisArray.Get(i)
		}
		thisArray.SetElements(newElements)
		return firstElement
	}))

	arrayProto.SetOwn("unshift", vm.NewNativeFunction(0, true, "unshift", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.NumberValue(0)
		}
		// Create new array with unshifted elements
		newElements := make([]vm.Value, 0, thisArray.Length()+len(args))
		// Add new elements first
		for i := 0; i < len(args); i++ {
			newElements = append(newElements, args[i])
		}
		// Add existing elements
		for i := 0; i < thisArray.Length(); i++ {
			newElements = append(newElements, thisArray.Get(i))
		}
		thisArray.SetElements(newElements)
		return vm.NumberValue(float64(thisArray.Length()))
	}))

	arrayProto.SetOwn("slice", vm.NewNativeFunction(2, false, "slice", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.NewArray()
		}
		length := thisArray.Length()
		start := 0
		if len(args) >= 1 {
			start = int(args[0].ToFloat())
			if start < 0 {
				start = length + start
				if start < 0 {
					start = 0
				}
			} else if start > length {
				start = length
			}
		}
		end := length
		if len(args) >= 2 {
			end = int(args[1].ToFloat())
			if end < 0 {
				end = length + end
				if end < 0 {
					end = 0
				}
			} else if end > length {
				end = length
			}
		}
		if start >= end {
			return vm.NewArray()
		}
		elements := make([]vm.Value, end-start)
		for i := start; i < end; i++ {
			elements[i-start] = thisArray.Get(i)
		}
		return vm.NewArrayWithArgs(elements)
	}))

	arrayProto.SetOwn("splice", vm.NewNativeFunction(2, true, "splice", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.NewArray()
		}
		length := thisArray.Length()
		start := 0
		if len(args) >= 1 {
			start = int(args[0].ToFloat())
			if start < 0 {
				start = length + start
				if start < 0 {
					start = 0
				}
			} else if start > length {
				start = length
			}
		}
		deleteCount := length - start
		if len(args) >= 2 {
			deleteCount = int(args[1].ToFloat())
			if deleteCount < 0 {
				deleteCount = 0
			} else if deleteCount > length-start {
				deleteCount = length - start
			}
		}
		// Create array with deleted elements
		deleted := vm.NewArray()
		for i := 0; i < deleteCount; i++ {
			deleted.AsArray().Append(thisArray.Get(start + i))
		}
		// Remove elements and insert new ones
		// First collect items to insert
		itemsToInsert := make([]vm.Value, 0)
		for i := 2; i < len(args); i++ {
			itemsToInsert = append(itemsToInsert, args[i])
		}
		// Perform splice operation manually
		// Create new elements array
		newElements := make([]vm.Value, 0)
		// Add elements before start
		for i := 0; i < start; i++ {
			newElements = append(newElements, thisArray.Get(i))
		}
		// Add items to insert
		for _, item := range itemsToInsert {
			newElements = append(newElements, item)
		}
		// Add elements after deleted section
		for i := start + deleteCount; i < length; i++ {
			newElements = append(newElements, thisArray.Get(i))
		}
		thisArray.SetElements(newElements)
		return deleted
	}))

	arrayProto.SetOwn("concat", vm.NewNativeFunction(0, true, "concat", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.NewArray()
		}
		result := vm.NewArray()
		// Add elements from this array
		for i := 0; i < thisArray.Length(); i++ {
			result.AsArray().Append(thisArray.Get(i))
		}
		// Add elements from arguments
		for i := 0; i < len(args); i++ {
			if otherArray := args[i].AsArray(); otherArray != nil {
				// If it's an array, add each element
				for j := 0; j < otherArray.Length(); j++ {
					result.AsArray().Append(otherArray.Get(j))
				}
			} else {
				// If it's not an array, add the element itself
				result.AsArray().Append(args[i])
			}
		}
		return result
	}))

	arrayProto.SetOwn("join", vm.NewNativeFunction(1, false, "join", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.NewString("")
		}
		separator := ","
		if len(args) >= 1 {
			separator = args[0].ToString()
		}
		// Build joined string
		if thisArray.Length() == 0 {
			return vm.NewString("")
		}
		result := thisArray.Get(0).ToString()
		for i := 1; i < thisArray.Length(); i++ {
			result += separator + thisArray.Get(i).ToString()
		}
		return vm.NewString(result)
	}))

	arrayProto.SetOwn("reverse", vm.NewNativeFunction(0, false, "reverse", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vmInstance.GetThis()
		}
		length := thisArray.Length()
		// Reverse elements in place
		for i := 0; i < length/2; i++ {
			j := length - 1 - i
			left := thisArray.Get(i)
			right := thisArray.Get(j)
			thisArray.Set(i, right)
			thisArray.Set(j, left)
		}
		return vmInstance.GetThis() // Return the same array
	}))

	arrayProto.SetOwn("sort", vm.NewNativeFunction(1, false, "sort", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vmInstance.GetThis()
		}
		length := thisArray.Length()
		if length <= 1 {
			return vmInstance.GetThis()
		}
		// Extract elements to slice
		elements := make([]vm.Value, length)
		for i := 0; i < length; i++ {
			elements[i] = thisArray.Get(i)
		}
		// Simple bubble sort with string comparison
		for i := 0; i < length-1; i++ {
			for j := 0; j < length-i-1; j++ {
				if elements[j].ToString() > elements[j+1].ToString() {
					elements[j], elements[j+1] = elements[j+1], elements[j]
				}
			}
		}
		// Set sorted elements back
		thisArray.SetElements(elements)
		return vmInstance.GetThis() // Return the same array
	}))

	arrayProto.SetOwn("indexOf", vm.NewNativeFunction(2, false, "indexOf", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || len(args) < 1 {
			return vm.NumberValue(-1)
		}
		searchElement := args[0]
		fromIndex := 0
		if len(args) >= 2 {
			fromIndex = int(args[1].ToFloat())
			if fromIndex < 0 {
				fromIndex = thisArray.Length() + fromIndex
				if fromIndex < 0 {
					fromIndex = 0
				}
			}
		}
		for i := fromIndex; i < thisArray.Length(); i++ {
			if thisArray.Get(i).Is(searchElement) {
				return vm.NumberValue(float64(i))
			}
		}
		return vm.NumberValue(-1)
	}))

	arrayProto.SetOwn("lastIndexOf", vm.NewNativeFunction(2, false, "lastIndexOf", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || len(args) < 1 {
			return vm.NumberValue(-1)
		}
		searchElement := args[0]
		fromIndex := thisArray.Length() - 1
		if len(args) >= 2 {
			fromIndex = int(args[1].ToFloat())
			if fromIndex < 0 {
				fromIndex = thisArray.Length() + fromIndex
			} else if fromIndex >= thisArray.Length() {
				fromIndex = thisArray.Length() - 1
			}
		}
		for i := fromIndex; i >= 0; i-- {
			if thisArray.Get(i).Is(searchElement) {
				return vm.NumberValue(float64(i))
			}
		}
		return vm.NumberValue(-1)
	}))

	arrayProto.SetOwn("includes", vm.NewNativeFunction(2, false, "includes", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || len(args) < 1 {
			return vm.BooleanValue(false)
		}
		searchElement := args[0]
		fromIndex := 0
		if len(args) >= 2 {
			fromIndex = int(args[1].ToFloat())
			if fromIndex < 0 {
				fromIndex = thisArray.Length() + fromIndex
				if fromIndex < 0 {
					fromIndex = 0
				}
			}
		}
		for i := fromIndex; i < thisArray.Length(); i++ {
			if thisArray.Get(i).Is(searchElement) {
				return vm.BooleanValue(true)
			}
		}
		return vm.BooleanValue(false)
	}))

	arrayProto.SetOwn("find", vm.NewNativeFunction(1, false, "find", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || len(args) < 1 {
			return vm.Undefined
		}
		callback := args[0]
		if !callback.IsCallable() {
			return vm.Undefined
		}
		for i := 0; i < thisArray.Length(); i++ {
			element := thisArray.Get(i)
			result, _ := vmInstance.CallFunctionDirectly(callback, vm.Undefined, []vm.Value{element, vm.NumberValue(float64(i)), vmInstance.GetThis()})
			if result.IsTruthy() {
				return element
			}
		}
		return vm.Undefined
	}))

	arrayProto.SetOwn("findIndex", vm.NewNativeFunction(1, false, "findIndex", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || len(args) < 1 {
			return vm.NumberValue(-1)
		}
		callback := args[0]
		if !callback.IsCallable() {
			return vm.NumberValue(-1)
		}
		for i := 0; i < thisArray.Length(); i++ {
			element := thisArray.Get(i)
			result, _ := vmInstance.CallFunctionDirectly(callback, vm.Undefined, []vm.Value{element, vm.NumberValue(float64(i)), vmInstance.GetThis()})
			if result.IsTruthy() {
				return vm.NumberValue(float64(i))
			}
		}
		return vm.NumberValue(-1)
	}))

	arrayProto.SetOwn("filter", vm.NewNativeFunction(1, false, "filter", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || len(args) < 1 {
			return vm.NewArray()
		}
		callback := args[0]
		if !callback.IsCallable() {
			return vm.NewArray()
		}
		result := vm.NewArray()
		for i := 0; i < thisArray.Length(); i++ {
			element := thisArray.Get(i)
			test, _ := vmInstance.CallFunctionDirectly(callback, vm.Undefined, []vm.Value{element, vm.NumberValue(float64(i)), vmInstance.GetThis()})
			if test.IsTruthy() {
				result.AsArray().Append(element)
			}
		}
		return result
	}))

	arrayProto.SetOwn("map", vm.NewNativeFunction(1, false, "map", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || len(args) < 1 {
			return vm.NewArray()
		}
		callback := args[0]
		if !callback.IsCallable() {
			return vm.NewArray()
		}
		result := vm.NewArray()
		for i := 0; i < thisArray.Length(); i++ {
			element := thisArray.Get(i)
			mappedValue, _ := vmInstance.CallFunctionDirectly(callback, vm.Undefined, []vm.Value{element, vm.NumberValue(float64(i)), vmInstance.GetThis()})
			result.AsArray().Append(mappedValue)
		}
		return result
	}))

	arrayProto.SetOwn("forEach", vm.NewNativeFunction(1, false, "forEach", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || len(args) < 1 {
			return vm.Undefined
		}
		callback := args[0]
		if !callback.IsCallable() {
			return vm.Undefined
		}
		for i := 0; i < thisArray.Length(); i++ {
			element := thisArray.Get(i)
			vmInstance.CallFunctionDirectly(callback, vm.Undefined, []vm.Value{element, vm.NumberValue(float64(i)), vmInstance.GetThis()})
		}
		return vm.Undefined
	}))

	arrayProto.SetOwn("every", vm.NewNativeFunction(1, false, "every", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || len(args) < 1 {
			return vm.BooleanValue(true)
		}
		callback := args[0]
		if !callback.IsCallable() {
			return vm.BooleanValue(true)
		}
		for i := 0; i < thisArray.Length(); i++ {
			element := thisArray.Get(i)
			result, _ := vmInstance.CallFunctionDirectly(callback, vm.Undefined, []vm.Value{element, vm.NumberValue(float64(i)), vmInstance.GetThis()})
			if !result.IsTruthy() {
				return vm.BooleanValue(false)
			}
		}
		return vm.BooleanValue(true)
	}))

	arrayProto.SetOwn("some", vm.NewNativeFunction(1, false, "some", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || len(args) < 1 {
			return vm.BooleanValue(false)
		}
		callback := args[0]
		if !callback.IsCallable() {
			return vm.BooleanValue(false)
		}
		for i := 0; i < thisArray.Length(); i++ {
			element := thisArray.Get(i)
			result, _ := vmInstance.CallFunctionDirectly(callback, vm.Undefined, []vm.Value{element, vm.NumberValue(float64(i)), vmInstance.GetThis()})
			if result.IsTruthy() {
				return vm.BooleanValue(true)
			}
		}
		return vm.BooleanValue(false)
	}))

	arrayProto.SetOwn("reduce", vm.NewNativeFunction(2, false, "reduce", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || len(args) < 1 {
			return vm.Undefined
		}
		callback := args[0]
		if !callback.IsCallable() {
			return vm.Undefined
		}
		length := thisArray.Length()
		if length == 0 {
			if len(args) >= 2 {
				return args[1] // Return initial value
			}
			return vm.Undefined // Should throw TypeError
		}
		accumulator := vm.Undefined
		startIndex := 0
		if len(args) >= 2 {
			accumulator = args[1]
		} else {
			accumulator = thisArray.Get(0)
			startIndex = 1
		}
		for i := startIndex; i < length; i++ {
			element := thisArray.Get(i)
			accumulator, _ = vmInstance.CallFunctionDirectly(callback, vm.Undefined, []vm.Value{accumulator, element, vm.NumberValue(float64(i)), vmInstance.GetThis()})
		}
		return accumulator
	}))

	arrayProto.SetOwn("reduceRight", vm.NewNativeFunction(2, false, "reduceRight", func(args []vm.Value) vm.Value {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || len(args) < 1 {
			return vm.Undefined
		}
		callback := args[0]
		if !callback.IsCallable() {
			return vm.Undefined
		}
		length := thisArray.Length()
		if length == 0 {
			if len(args) >= 2 {
				return args[1] // Return initial value
			}
			return vm.Undefined // Should throw TypeError
		}
		accumulator := vm.Undefined
		startIndex := length - 1
		if len(args) >= 2 {
			accumulator = args[1]
		} else {
			accumulator = thisArray.Get(length - 1)
			startIndex = length - 2
		}
		for i := startIndex; i >= 0; i-- {
			element := thisArray.Get(i)
			accumulator, _ = vmInstance.CallFunctionDirectly(callback, vm.Undefined, []vm.Value{accumulator, element, vm.NumberValue(float64(i)), vmInstance.GetThis()})
		}
		return accumulator
	}))

	// Create Array constructor
	ctorWithProps := vm.NewNativeFunctionWithProps(-1, true, "Array", func(args []vm.Value) vm.Value {
		if len(args) == 0 {
			return vm.NewArray()
		}
		if len(args) == 1 {
			// If single argument is a number, create array with that length
			if args[0].IsNumber() {
				length := int(args[0].ToFloat())
				if length < 0 {
					return vm.NewArray() // Should throw RangeError in real JS
				}
				result := vm.NewArray()
				// Set length but don't populate with elements
				for i := 0; i < length; i++ {
					result.AsArray().Append(vm.Undefined)
				}
				return result
			}
		}
		// Multiple arguments or single non-number argument - create array with those elements
		return vm.NewArrayWithArgs(args)
	})

	// Add prototype property
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("prototype", vm.NewValueFromPlainObject(arrayProto))

	// Add static methods
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("isArray", vm.NewNativeFunction(1, false, "isArray", func(args []vm.Value) vm.Value {
		if len(args) < 1 {
			return vm.BooleanValue(false)
		}
		return vm.BooleanValue(args[0].Type() == vm.TypeArray)
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("from", vm.NewNativeFunction(2, false, "from", func(args []vm.Value) vm.Value {
		if len(args) < 1 {
			return vm.NewArray()
		}
		arrayLike := args[0]

		// If it's already an array, create a shallow copy
		if sourceArray := arrayLike.AsArray(); sourceArray != nil {
			result := vm.NewArray()
			for i := 0; i < sourceArray.Length(); i++ {
				element := sourceArray.Get(i)
				// Apply mapping function if provided
				if len(args) >= 2 && args[1].IsCallable() {
					mapFn := args[1]
					mappedValue, _ := vmInstance.CallFunctionDirectly(mapFn, vm.Undefined, []vm.Value{element, vm.NumberValue(float64(i))})
					result.AsArray().Append(mappedValue)
				} else {
					result.AsArray().Append(element)
				}
			}
			return result
		}

		// For non-arrays, try to treat as array-like (simplified implementation)
		// In a full implementation, this would handle iterables, strings, etc.
		return vm.NewArray()
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) vm.Value {
		return vm.NewArrayWithArgs(args)
	}))

	arrayCtor := ctorWithProps

	// Set Array prototype in VM
	vmInstance.ArrayPrototype = vm.NewValueFromPlainObject(arrayProto)

	// Register Array constructor as global
	return ctx.DefineGlobal("Array", arrayCtor)
}
