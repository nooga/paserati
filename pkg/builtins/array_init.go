package builtins

import (
	"errors"
	"fmt"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// toIntegerOrInfinityWithVM converts a value to an integer using proper ECMAScript semantics.
// It calls ToPrimitive if needed and propagates exceptions from valueOf/toString.
// Returns (result, nil) on success, (0, ErrVMUnwinding) if ToPrimitive threw, or a TypeError for Symbols.
func toIntegerOrInfinityWithVM(vmInstance *vm.VM, val vm.Value) (int, error) {
	// Check for Symbol first - cannot convert Symbol to number
	if val.Type() == vm.TypeSymbol {
		return 0, vmInstance.NewTypeError("Cannot convert a Symbol value to a number")
	}

	// For primitives, no ToPrimitive call is needed
	if !val.IsObject() && !val.IsCallable() {
		return int(val.ToFloat()), nil
	}

	// For objects, call ToPrimitive which may invoke valueOf/toString
	vmInstance.EnterHelperCall()
	primVal := vmInstance.ToPrimitive(val, "number")
	vmInstance.ExitHelperCall()

	// Check if ToPrimitive threw an exception
	if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
		return 0, ErrVMUnwinding
	}

	// Check if ToPrimitive returned a Symbol (from [Symbol.toPrimitive])
	if primVal.Type() == vm.TypeSymbol {
		return 0, vmInstance.NewTypeError("Cannot convert a Symbol value to a number")
	}

	return int(primVal.ToFloat()), nil
}

type ArrayInitializer struct{}

func (a *ArrayInitializer) Name() string {
	return "Array"
}

func (a *ArrayInitializer) Priority() int {
	return PriorityArray // 2 - After Object (0) and Function (1)
}

func (a *ArrayInitializer) InitTypes(ctx *TypeContext) error {
	// Create generic type parameter T for array methods
	tParam := &types.TypeParameter{Name: "T", Constraint: nil, Index: 0}
	tType := &types.TypeParameterType{Parameter: tParam}
	tArrayType := &types.ArrayType{ElementType: tType}

	// Create Array.prototype type with selective generic methods
	arrayProtoType := types.NewObjectType().
		WithProperty("length", types.Number).
		// Keep mutation methods non-generic for flexibility
		WithVariadicProperty("push", []types.Type{}, types.Number, &types.ArrayType{ElementType: types.Any}).
		WithProperty("pop", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("shift", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithVariadicProperty("unshift", []types.Type{}, types.Number, &types.ArrayType{ElementType: types.Any}).
		WithProperty("slice", a.createGenericMethod("slice", tParam,
			types.NewOptionalFunction([]types.Type{types.Number, types.Number}, tArrayType, []bool{true, true}))).
		// Keep concat non-generic for flexibility with different array types
		WithVariadicProperty("concat", []types.Type{}, &types.ArrayType{ElementType: types.Any}, &types.ArrayType{ElementType: types.Any}).
		WithProperty("join", types.NewSimpleFunction([]types.Type{types.String}, types.String)).
		WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("reverse", a.createGenericMethod("reverse", tParam,
			types.NewSimpleFunction([]types.Type{}, tArrayType))).
		WithProperty("indexOf", a.createGenericMethod("indexOf", tParam,
			types.NewOptionalFunction([]types.Type{tType, types.Number}, types.Number, []bool{false, true}))).
		WithProperty("lastIndexOf", a.createGenericMethod("lastIndexOf", tParam,
			types.NewOptionalFunction([]types.Type{tType, types.Number}, types.Number, []bool{false, true}))).
		WithProperty("includes", a.createGenericMethod("includes", tParam,
			types.NewOptionalFunction([]types.Type{tType, types.Number}, types.Boolean, []bool{false, true}))).
		// Make callback-based methods generic (these are the important ones!)
		WithProperty("find", a.createGenericMethod("find", tParam,
			types.NewSimpleFunction([]types.Type{
				types.NewOptionalFunction([]types.Type{tType, types.Number, tArrayType}, types.Boolean, []bool{false, true, true})},
				types.NewUnionType(tType, types.Undefined)))).
		WithProperty("findIndex", a.createGenericMethod("findIndex", tParam,
			types.NewSimpleFunction([]types.Type{
				types.NewOptionalFunction([]types.Type{tType, types.Number, tArrayType}, types.Boolean, []bool{false, true, true})},
				types.Number))).
		WithProperty("filter", a.createGenericMethod("filter", tParam,
			types.NewSimpleFunction([]types.Type{
				types.NewOptionalFunction([]types.Type{tType, types.Number, tArrayType}, types.Boolean, []bool{false, true, true})},
				tArrayType))).
		WithProperty("map", a.createGenericMapMethod(tParam)).
		WithProperty("forEach", a.createGenericMethod("forEach", tParam,
			types.NewSimpleFunction([]types.Type{
				types.NewOptionalFunction([]types.Type{tType, types.Number, tArrayType}, types.Undefined, []bool{false, true, true})},
				types.Undefined))).
		WithProperty("every", a.createGenericMethod("every", tParam,
			types.NewSimpleFunction([]types.Type{
				types.NewOptionalFunction([]types.Type{tType, types.Number, tArrayType}, types.Boolean, []bool{false, true, true})},
				types.Boolean))).
		WithProperty("some", a.createGenericMethod("some", tParam,
			types.NewSimpleFunction([]types.Type{
				types.NewOptionalFunction([]types.Type{tType, types.Number, tArrayType}, types.Boolean, []bool{false, true, true})},
				types.Boolean))).
		// Keep reduce non-generic for now since it's complex
		WithProperty("reduce", types.NewSimpleFunction([]types.Type{types.NewSimpleFunction([]types.Type{types.Any, types.Any, types.Number, &types.ArrayType{ElementType: types.Any}}, types.Any), types.Any}, types.Any)).
		WithProperty("reduceRight", types.NewSimpleFunction([]types.Type{types.NewSimpleFunction([]types.Type{types.Any, types.Any, types.Number, &types.ArrayType{ElementType: types.Any}}, types.Any), types.Any}, types.Any))

	// Add Symbol.iterator to array prototype type to make arrays iterable
	// Get the Iterator<T> generic type if available
	if iteratorType, found := ctx.GetType("Iterator"); found {
		if iteratorGeneric, ok := iteratorType.(*types.GenericType); ok {
			// Create Iterator<T> type for arrays
			iteratorOfT := &types.InstantiatedType{
				Generic:       iteratorGeneric,
				TypeArguments: []types.Type{tType},
			}
			// Add [Symbol.iterator](): Iterator<T> method (computed symbol key in types)
			arrayProtoType = arrayProtoType.WithProperty("__COMPUTED_PROPERTY__",
				a.createGenericMethod("[Symbol.iterator]", tParam,
					types.NewSimpleFunction([]types.Type{}, iteratorOfT.Substitute())))
		}
	}

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
	arrayProto.SetOwnNonEnumerable("push", vm.NewNativeFunction(1, true, "push", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.NumberValue(0), nil
		}
		for i := 0; i < len(args); i++ {
			thisArray.Append(args[i])
		}
		return vm.NumberValue(float64(thisArray.Length())), nil
	}))

	arrayProto.SetOwnNonEnumerable("pop", vm.NewNativeFunction(0, false, "pop", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || thisArray.Length() == 0 {
			return vm.Undefined, nil
		}
		lastIndex := thisArray.Length() - 1
		lastElement := thisArray.Get(lastIndex)
		thisArray.SetLength(lastIndex)
		return lastElement, nil
	}))

	arrayProto.SetOwnNonEnumerable("shift", vm.NewNativeFunction(0, false, "shift", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || thisArray.Length() == 0 {
			return vm.Undefined, nil
		}
		firstElement := thisArray.Get(0)
		// Shift all elements left
		newElements := make([]vm.Value, thisArray.Length()-1)
		for i := 1; i < thisArray.Length(); i++ {
			newElements[i-1] = thisArray.Get(i)
		}
		thisArray.SetElements(newElements)
		return firstElement, nil
	}))

	arrayProto.SetOwnNonEnumerable("unshift", vm.NewNativeFunction(1, true, "unshift", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.NumberValue(0), nil
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
		return vm.NumberValue(float64(thisArray.Length())), nil
	}))

	arrayProto.SetOwnNonEnumerable("slice", vm.NewNativeFunction(2, false, "slice", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.NewArray(), nil
		}
		length := thisArray.Length()
		start := 0
		if len(args) >= 1 && !args[0].IsUndefined() {
			var err error
			start, err = toIntegerOrInfinityWithVM(vmInstance, args[0])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
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
		if len(args) >= 2 && !args[1].IsUndefined() {
			var err error
			end, err = toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
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
			return vm.NewArray(), nil
		}
		elements := make([]vm.Value, end-start)
		for i := start; i < end; i++ {
			elements[i-start] = thisArray.Get(i)
		}
		return vm.NewArrayWithArgs(elements), nil
	}))

	arrayProto.SetOwnNonEnumerable("splice", vm.NewNativeFunction(2, true, "splice", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.NewArray(), nil
		}
		length := thisArray.Length()
		start := 0
		if len(args) >= 1 {
			var err error
			start, err = toIntegerOrInfinityWithVM(vmInstance, args[0])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
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
			var err error
			deleteCount, err = toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
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
		return deleted, nil
	}))

	arrayProto.SetOwnNonEnumerable("concat", vm.NewNativeFunction(1, true, "concat", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.NewArray(), nil
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
		return result, nil
	}))

	arrayProto.SetOwnNonEnumerable("join", vm.NewNativeFunction(1, false, "join", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.NewString(""), nil
		}
		separator := ","
		if len(args) >= 1 {
			separator = args[0].ToString()
		}
		// Build joined string
		if thisArray.Length() == 0 {
			return vm.NewString(""), nil
		}
		result := thisArray.Get(0).ToString()
		for i := 1; i < thisArray.Length(); i++ {
			result += separator + thisArray.Get(i).ToString()
		}
		return vm.NewString(result), nil
	}))

	arrayProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		// Per ECMAScript spec, Array.prototype.toString is equivalent to calling join() with no arguments
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.NewString(""), nil
		}
		// Build comma-separated string (same as join with default separator)
		if thisArray.Length() == 0 {
			return vm.NewString(""), nil
		}
		result := thisArray.Get(0).ToString()
		for i := 1; i < thisArray.Length(); i++ {
			result += "," + thisArray.Get(i).ToString()
		}
		return vm.NewString(result), nil
	}))

	arrayProto.SetOwnNonEnumerable("reverse", vm.NewNativeFunction(0, false, "reverse", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vmInstance.GetThis(), nil
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
		return vmInstance.GetThis(), nil // Return the same array
	}))

	arrayProto.SetOwnNonEnumerable("sort", vm.NewNativeFunction(1, false, "sort", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vmInstance.GetThis(), nil
		}
		length := thisArray.Length()
		if length <= 1 {
			return vmInstance.GetThis(), nil
		}
		// Extract elements to slice
		elements := make([]vm.Value, length)
		for i := 0; i < length; i++ {
			elements[i] = thisArray.Get(i)
		}

		// Get comparator function if provided
		var compareFn vm.Value
		if len(args) > 0 && args[0].IsCallable() {
			compareFn = args[0]
		}

		// Simple bubble sort (not efficient but correct)
		for i := 0; i < length-1; i++ {
			for j := 0; j < length-i-1; j++ {
				var shouldSwap bool
				if compareFn.IsCallable() {
					// Use the comparator function
					result, err := vmInstance.Call(compareFn, vm.Undefined, []vm.Value{elements[j], elements[j+1]})
					if err != nil {
						return vm.Undefined, err
					}
					// Per ECMAScript: compareFn(a, b) > 0 means a should come after b
					shouldSwap = result.ToFloat() > 0
				} else {
					// Default: string comparison per ECMAScript spec
					shouldSwap = elements[j].ToString() > elements[j+1].ToString()
				}
				if shouldSwap {
					elements[j], elements[j+1] = elements[j+1], elements[j]
				}
			}
		}
		// Set sorted elements back
		thisArray.SetElements(elements)
		return vmInstance.GetThis(), nil // Return the same array
	}))

	arrayProto.SetOwnNonEnumerable("indexOf", vm.NewNativeFunction(1, false, "indexOf", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.NumberValue(-1), nil
		}
		// If no argument, search for undefined
		var searchElement vm.Value
		if len(args) >= 1 {
			searchElement = args[0]
		} else {
			searchElement = vm.Undefined
		}
		length := thisArray.Length()
		if length == 0 {
			return vm.NumberValue(-1), nil
		}
		fromIndex := 0
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			var err error
			fromIndex, err = toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
			if fromIndex < 0 {
				fromIndex = length + fromIndex
				if fromIndex < 0 {
					fromIndex = 0
				}
			}
		}
		// ECMAScript spec: indexOf uses Strict Equality (===), not SameValueZero
		// This means NaN !== NaN, so indexOf(NaN) should return -1
		for i := fromIndex; i < length; i++ {
			// Skip holes in sparse arrays
			if !thisArray.HasIndex(i) {
				continue
			}
			if thisArray.Get(i).StrictlyEquals(searchElement) {
				return vm.NumberValue(float64(i)), nil
			}
		}
		return vm.NumberValue(-1), nil
	}))

	arrayProto.SetOwnNonEnumerable("lastIndexOf", vm.NewNativeFunction(1, false, "lastIndexOf", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.NumberValue(-1), nil
		}
		// If no argument, search for undefined
		var searchElement vm.Value
		if len(args) >= 1 {
			searchElement = args[0]
		} else {
			searchElement = vm.Undefined
		}
		length := thisArray.Length()
		if length == 0 {
			return vm.NumberValue(-1), nil
		}
		fromIndex := length - 1
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			var err error
			fromIndex, err = toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
			if fromIndex < 0 {
				fromIndex = length + fromIndex
			} else if fromIndex >= length {
				fromIndex = length - 1
			}
		}
		// ECMAScript spec: lastIndexOf uses Strict Equality (===), not SameValueZero
		// This means NaN !== NaN, so lastIndexOf(NaN) should return -1
		for i := fromIndex; i >= 0; i-- {
			// Skip holes in sparse arrays
			if !thisArray.HasIndex(i) {
				continue
			}
			if thisArray.Get(i).StrictlyEquals(searchElement) {
				return vm.NumberValue(float64(i)), nil
			}
		}
		return vm.NumberValue(-1), nil
	}))

	arrayProto.SetOwnNonEnumerable("includes", vm.NewNativeFunction(1, false, "includes", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.BooleanValue(false), nil
		}
		// If no argument, search for undefined
		var searchElement vm.Value
		if len(args) >= 1 {
			searchElement = args[0]
		} else {
			searchElement = vm.Undefined
		}
		length := thisArray.Length()
		if length == 0 {
			return vm.BooleanValue(false), nil
		}
		fromIndex := 0
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			var err error
			fromIndex, err = toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
			if fromIndex < 0 {
				fromIndex = length + fromIndex
				if fromIndex < 0 {
					fromIndex = 0
				}
			}
		}
		// ECMAScript spec: includes uses SameValueZero, so NaN === NaN (Is() is correct)
		// Note: includes DOES check holes and finds undefined in them (unlike indexOf)
		for i := fromIndex; i < length; i++ {
			if thisArray.Get(i).Is(searchElement) {
				return vm.BooleanValue(true), nil
			}
		}
		return vm.BooleanValue(false), nil
	}))

	arrayProto.SetOwnNonEnumerable("find", vm.NewNativeFunction(1, false, "find", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || len(args) < 1 {
			return vm.Undefined, nil
		}
		callback := args[0]
		if !callback.IsCallable() {
			return vm.Undefined, nil
		}
		for i := 0; i < thisArray.Length(); i++ {
			element := thisArray.Get(i)
			result, err := vmInstance.Call(callback, vm.Undefined, []vm.Value{element, vm.NumberValue(float64(i)), vmInstance.GetThis()})
			if err != nil {
				return vm.Undefined, err
			}
			if result.IsTruthy() {
				return element, nil
			}
		}
		return vm.Undefined, nil
	}))

	arrayProto.SetOwnNonEnumerable("findIndex", vm.NewNativeFunction(1, false, "findIndex", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil || len(args) < 1 {
			return vm.NumberValue(-1), nil
		}
		callback := args[0]
		if !callback.IsCallable() {
			return vm.NumberValue(-1), nil
		}
		for i := 0; i < thisArray.Length(); i++ {
			element := thisArray.Get(i)
			result, err := vmInstance.Call(callback, vm.Undefined, []vm.Value{element, vm.NumberValue(float64(i)), vmInstance.GetThis()})
			if err != nil {
				return vm.NumberValue(-1), err
			}
			if result.IsTruthy() {
				return vm.NumberValue(float64(i)), nil
			}
		}
		return vm.NumberValue(-1), nil
	}))

	arrayProto.SetOwnNonEnumerable("filter", vm.NewNativeFunction(1, false, "filter", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.filter called on null or undefined")
		}

		// 2. Let len be ? LengthOfArrayLike(O). - MUST access length BEFORE checking callback
		var length int
		if arr := thisVal.AsArray(); arr != nil {
			length = arr.Length()
		} else if po := thisVal.AsPlainObject(); po != nil {
			if lv, ok := po.Get("length"); ok {
				length = int(lv.ToFloat())
				if length < 0 {
					length = 0
				}
			}
		}

		// 3. If IsCallable(callbackfn) is false, throw a TypeError exception.
		var callback vm.Value
		if len(args) >= 1 {
			callback = args[0]
		} else {
			callback = vm.Undefined
		}
		if !callback.IsCallable() {
			callbackStr := "undefined"
			if callback.Type() != vm.TypeUndefined {
				callbackStr = callback.ToString()
			}
			return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", callbackStr))
		}

		// Get thisArg (second argument to filter)
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}

		result := vm.NewArray()

		// Support both arrays and array-like objects (with sparse array support)
		if arr := thisVal.AsArray(); arr != nil {
			for i := 0; i < length; i++ {
				// Only call callback for indices that actually exist (sparse array support)
				if arr.HasIndex(i) {
					element := arr.Get(i)
					test, err := vmInstance.Call(callback, thisArg, []vm.Value{element, vm.NumberValue(float64(i)), thisVal})
					if err != nil {
						return vm.NewArray(), err
					}
					if test.IsTruthy() {
						result.AsArray().Append(element)
					}
				}
			}
		} else if po := thisVal.AsPlainObject(); po != nil {
			for i := 0; i < length; i++ {
				key := fmt.Sprintf("%d", i)
				// Only call callback for indices that actually exist
				if _, ok := po.GetOwn(key); ok {
					elem, _ := po.Get(key)
					test, err := vmInstance.Call(callback, thisArg, []vm.Value{elem, vm.NumberValue(float64(i)), thisVal})
					if err != nil {
						return vm.NewArray(), err
					}
					if test.IsTruthy() {
						result.AsArray().Append(elem)
					}
				}
			}
		}
		return result, nil
	}))

	arrayProto.SetOwnNonEnumerable("map", vm.NewNativeFunction(1, false, "map", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.map called on null or undefined")
		}

		// 2. Let len be ? LengthOfArrayLike(O). - MUST access length BEFORE checking callback
		var length int
		if arr := thisVal.AsArray(); arr != nil {
			length = arr.Length()
		} else if po := thisVal.AsPlainObject(); po != nil {
			if lv, ok := po.Get("length"); ok {
				length = int(lv.ToFloat())
				if length < 0 {
					length = 0
				}
			}
		}

		// 3. If IsCallable(callbackfn) is false, throw a TypeError exception.
		var callback vm.Value
		if len(args) >= 1 {
			callback = args[0]
		} else {
			callback = vm.Undefined
		}
		if !callback.IsCallable() {
			callbackStr := "undefined"
			if callback.Type() != vm.TypeUndefined {
				callbackStr = callback.ToString()
			}
			return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", callbackStr))
		}

		// Get thisArg (second argument to map)
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}

		// Create result array with same length (for sparse array support)
		result := vm.NewArray()
		resultArr := result.AsArray()
		resultArr.SetLength(length)

		if arr := thisVal.AsArray(); arr != nil {
			for i := 0; i < length; i++ {
				// Only call callback for indices that actually exist (sparse array support)
				if arr.HasIndex(i) {
					element := arr.Get(i)
					mappedValue, err := vmInstance.Call(callback, thisArg, []vm.Value{element, vm.NumberValue(float64(i)), thisVal})
					if err != nil {
						return vm.Undefined, err
					}
					resultArr.Set(i, mappedValue)
				}
			}
			return result, nil
		}

		// Array-like: { length: N, 0: ..., 1: ..., ... }
		if po := thisVal.AsPlainObject(); po != nil {
			for i := 0; i < length; i++ {
				key := fmt.Sprintf("%d", i)
				// Only call callback for indices that actually exist
				if _, ok := po.GetOwn(key); ok {
					elem, _ := po.Get(key)
					mappedValue, err := vmInstance.Call(callback, thisArg, []vm.Value{elem, vm.NumberValue(float64(i)), thisVal})
					if err != nil {
						return vm.Undefined, err
					}
					resultArr.Set(i, mappedValue)
				}
			}
			return result, nil
		}

		// Non-array-like: return empty array
		return result, nil
	}))

	arrayProto.SetOwnNonEnumerable("forEach", vm.NewNativeFunction(1, false, "forEach", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.forEach called on null or undefined")
		}

		// 2. Let len be ? LengthOfArrayLike(O). - MUST access length BEFORE checking callback
		var length int
		if arr := thisVal.AsArray(); arr != nil {
			length = arr.Length()
		} else if po := thisVal.AsPlainObject(); po != nil {
			if lv, ok := po.Get("length"); ok {
				length = int(lv.ToFloat())
				if length < 0 {
					length = 0
				}
			}
		}

		// 3. If IsCallable(callbackfn) is false, throw a TypeError exception.
		var callback vm.Value
		if len(args) >= 1 {
			callback = args[0]
		} else {
			callback = vm.Undefined
		}
		if !callback.IsCallable() {
			callbackStr := "undefined"
			if callback.Type() != vm.TypeUndefined {
				callbackStr = callback.ToString()
			}
			return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", callbackStr))
		}

		// Get thisArg (second argument to forEach)
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}

		// Support both arrays and array-like objects (with sparse array support)
		if arr := thisVal.AsArray(); arr != nil {
			for i := 0; i < length; i++ {
				// Only call callback for indices that actually exist (sparse array support)
				if arr.HasIndex(i) {
					element := arr.Get(i)
					_, err := vmInstance.Call(callback, thisArg, []vm.Value{element, vm.NumberValue(float64(i)), thisVal})
					if err != nil {
						return vm.Undefined, err
					}
				}
			}
		} else if po := thisVal.AsPlainObject(); po != nil {
			for i := 0; i < length; i++ {
				key := fmt.Sprintf("%d", i)
				// Only call callback for indices that actually exist
				if _, ok := po.GetOwn(key); ok {
					elem, _ := po.Get(key)
					_, err := vmInstance.Call(callback, thisArg, []vm.Value{elem, vm.NumberValue(float64(i)), thisVal})
					if err != nil {
						return vm.Undefined, err
					}
				}
			}
		}
		return vm.Undefined, nil
	}))

	arrayProto.SetOwnNonEnumerable("every", vm.NewNativeFunction(1, false, "every", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.every called on null or undefined")
		}

		// 2. Let len be ? LengthOfArrayLike(O). - MUST access length BEFORE checking callback
		var length int
		if arr := thisVal.AsArray(); arr != nil {
			length = arr.Length()
		} else if po := thisVal.AsPlainObject(); po != nil {
			if lv, ok := po.Get("length"); ok {
				length = int(lv.ToFloat())
				if length < 0 {
					length = 0
				}
			}
		}

		// 3. If IsCallable(callbackfn) is false, throw a TypeError exception.
		var callback vm.Value
		if len(args) >= 1 {
			callback = args[0]
		} else {
			callback = vm.Undefined
		}
		if !callback.IsCallable() {
			callbackStr := "undefined"
			if callback.Type() != vm.TypeUndefined {
				callbackStr = callback.ToString()
			}
			return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", callbackStr))
		}

		// Get thisArg (second argument to every)
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}

		// Support both arrays and array-like objects (with sparse array support)
		if arr := thisVal.AsArray(); arr != nil {
			for i := 0; i < length; i++ {
				// Only call callback for indices that actually exist (sparse array support)
				if arr.HasIndex(i) {
					element := arr.Get(i)
					result, err := vmInstance.Call(callback, thisArg, []vm.Value{element, vm.NumberValue(float64(i)), thisVal})
					if err != nil {
						return vm.BooleanValue(false), err
					}
					if !result.IsTruthy() {
						return vm.BooleanValue(false), nil
					}
				}
			}
		} else if po := thisVal.AsPlainObject(); po != nil {
			for i := 0; i < length; i++ {
				key := fmt.Sprintf("%d", i)
				// Only call callback for indices that actually exist
				if _, ok := po.GetOwn(key); ok {
					elem, _ := po.Get(key)
					result, err := vmInstance.Call(callback, thisArg, []vm.Value{elem, vm.NumberValue(float64(i)), thisVal})
					if err != nil {
						return vm.BooleanValue(false), err
					}
					if !result.IsTruthy() {
						return vm.BooleanValue(false), nil
					}
				}
			}
		}
		return vm.BooleanValue(true), nil
	}))

	arrayProto.SetOwnNonEnumerable("some", vm.NewNativeFunction(1, false, "some", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.some called on null or undefined")
		}

		// 2. Let len be ? LengthOfArrayLike(O). - MUST access length BEFORE checking callback
		var length int
		if arr := thisVal.AsArray(); arr != nil {
			length = arr.Length()
		} else if po := thisVal.AsPlainObject(); po != nil {
			if lv, ok := po.Get("length"); ok {
				length = int(lv.ToFloat())
				if length < 0 {
					length = 0
				}
			}
		}

		// 3. If IsCallable(callbackfn) is false, throw a TypeError exception.
		var callback vm.Value
		if len(args) >= 1 {
			callback = args[0]
		} else {
			callback = vm.Undefined
		}
		if !callback.IsCallable() {
			callbackStr := "undefined"
			if callback.Type() != vm.TypeUndefined {
				callbackStr = callback.ToString()
			}
			return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", callbackStr))
		}

		// Get thisArg (second argument to some)
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}

		// Support both arrays and array-like objects (with sparse array support)
		if arr := thisVal.AsArray(); arr != nil {
			for i := 0; i < length; i++ {
				// Only call callback for indices that actually exist (sparse array support)
				if arr.HasIndex(i) {
					element := arr.Get(i)
					result, err := vmInstance.Call(callback, thisArg, []vm.Value{element, vm.NumberValue(float64(i)), thisVal})
					if err != nil {
						return vm.BooleanValue(false), err
					}
					if result.IsTruthy() {
						return vm.BooleanValue(true), nil
					}
				}
			}
		} else if po := thisVal.AsPlainObject(); po != nil {
			for i := 0; i < length; i++ {
				key := fmt.Sprintf("%d", i)
				// Only call callback for indices that actually exist
				if _, ok := po.GetOwn(key); ok {
					elem, _ := po.Get(key)
					result, err := vmInstance.Call(callback, thisArg, []vm.Value{elem, vm.NumberValue(float64(i)), thisVal})
					if err != nil {
						return vm.BooleanValue(false), err
					}
					if result.IsTruthy() {
						return vm.BooleanValue(true), nil
					}
				}
			}
		}
		return vm.BooleanValue(false), nil
	}))

	arrayProto.SetOwnNonEnumerable("reduce", vm.NewNativeFunction(1, false, "reduce", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.reduce called on null or undefined")
		}

		// 2. Let len be ? LengthOfArrayLike(O). - MUST access length BEFORE checking callback
		var length int
		if arr := thisVal.AsArray(); arr != nil {
			length = arr.Length()
		} else if po := thisVal.AsPlainObject(); po != nil {
			if lv, ok := po.Get("length"); ok {
				length = int(lv.ToFloat())
				if length < 0 {
					length = 0
				}
			}
		}

		// 3. If IsCallable(callbackfn) is false, throw a TypeError exception.
		var callback vm.Value
		if len(args) >= 1 {
			callback = args[0]
		} else {
			callback = vm.Undefined
		}
		if !callback.IsCallable() {
			callbackStr := "undefined"
			if callback.Type() != vm.TypeUndefined {
				callbackStr = callback.ToString()
			}
			return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", callbackStr))
		}

		// 5. If len = 0 and initialValue is not present, throw a TypeError exception.
		if length == 0 && len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Reduce of empty array with no initial value")
		}

		accumulator := vm.Undefined
		startIndex := 0
		if len(args) >= 2 {
			accumulator = args[1]
		} else if arr := thisVal.AsArray(); arr != nil {
			accumulator = arr.Get(0)
			startIndex = 1
		} else if po := thisVal.AsPlainObject(); po != nil {
			if v, ok := po.Get("0"); ok {
				accumulator = v
			}
			startIndex = 1
		}

		// Iterate
		if arr := thisVal.AsArray(); arr != nil {
			for i := startIndex; i < length; i++ {
				element := arr.Get(i)
				var err error
				accumulator, err = vmInstance.Call(callback, vm.Undefined, []vm.Value{accumulator, element, vm.NumberValue(float64(i)), thisVal})
				if err != nil {
					return vm.Undefined, err
				}
			}
		} else if po := thisVal.AsPlainObject(); po != nil {
			for i := startIndex; i < length; i++ {
				key := fmt.Sprintf("%d", i)
				var elem vm.Value = vm.Undefined
				if v, ok := po.Get(key); ok {
					elem = v
				}
				var err error
				accumulator, err = vmInstance.Call(callback, vm.Undefined, []vm.Value{accumulator, elem, vm.NumberValue(float64(i)), thisVal})
				if err != nil {
					return vm.Undefined, err
				}
			}
		}
		return accumulator, nil
	}))

	arrayProto.SetOwnNonEnumerable("reduceRight", vm.NewNativeFunction(1, false, "reduceRight", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.reduceRight called on null or undefined")
		}

		// 2. Let len be ? LengthOfArrayLike(O). - MUST access length BEFORE checking callback
		var length int
		if arr := thisVal.AsArray(); arr != nil {
			length = arr.Length()
		} else if po := thisVal.AsPlainObject(); po != nil {
			if lv, ok := po.Get("length"); ok {
				length = int(lv.ToFloat())
				if length < 0 {
					length = 0
				}
			}
		}

		// 3. If IsCallable(callbackfn) is false, throw a TypeError exception.
		var callback vm.Value
		if len(args) >= 1 {
			callback = args[0]
		} else {
			callback = vm.Undefined
		}
		if !callback.IsCallable() {
			callbackStr := "undefined"
			if callback.Type() != vm.TypeUndefined {
				callbackStr = callback.ToString()
			}
			return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", callbackStr))
		}

		// 5. If len = 0 and initialValue is not present, throw a TypeError exception.
		if length == 0 && len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Reduce of empty array with no initial value")
		}

		accumulator := vm.Undefined
		startIndex := length - 1
		if len(args) >= 2 {
			accumulator = args[1]
		} else if arr := thisVal.AsArray(); arr != nil {
			accumulator = arr.Get(length - 1)
			startIndex = length - 2
		} else if po := thisVal.AsPlainObject(); po != nil {
			key := fmt.Sprintf("%d", length-1)
			if v, ok := po.Get(key); ok {
				accumulator = v
			}
			startIndex = length - 2
		}

		// Iterate backwards
		if arr := thisVal.AsArray(); arr != nil {
			for i := startIndex; i >= 0; i-- {
				element := arr.Get(i)
				var err error
				accumulator, err = vmInstance.Call(callback, vm.Undefined, []vm.Value{accumulator, element, vm.NumberValue(float64(i)), thisVal})
				if err != nil {
					return vm.Undefined, err
				}
			}
		} else if po := thisVal.AsPlainObject(); po != nil {
			for i := startIndex; i >= 0; i-- {
				key := fmt.Sprintf("%d", i)
				var elem vm.Value = vm.Undefined
				if v, ok := po.Get(key); ok {
					elem = v
				}
				var err error
				accumulator, err = vmInstance.Call(callback, vm.Undefined, []vm.Value{accumulator, elem, vm.NumberValue(float64(i)), thisVal})
				if err != nil {
					return vm.Undefined, err
				}
			}
		}
		return accumulator, nil
	}))

	// Array.prototype.at - relative indexing access
	arrayProto.SetOwnNonEnumerable("at", vm.NewNativeFunction(1, false, "at", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.at called on null or undefined")
		}

		// Get length
		var length int
		if arr := thisVal.AsArray(); arr != nil {
			length = arr.Length()
		} else if po := thisVal.AsPlainObject(); po != nil {
			if lv, ok := po.Get("length"); ok {
				length = int(lv.ToFloat())
			}
		}

		// 3. Let relativeIndex be ? ToIntegerOrInfinity(index).
		var relativeIndex int
		if len(args) >= 1 {
			var err error
			relativeIndex, err = toIntegerOrInfinityWithVM(vmInstance, args[0])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
		}

		// 4. If relativeIndex ≥ 0, let k be relativeIndex. Else let k be len + relativeIndex.
		var k int
		if relativeIndex >= 0 {
			k = relativeIndex
		} else {
			k = length + relativeIndex
		}

		// 5. If k < 0 or k ≥ len, return undefined.
		if k < 0 || k >= length {
			return vm.Undefined, nil
		}

		// 6. Return ? Get(O, ! ToString(k)).
		if arr := thisVal.AsArray(); arr != nil {
			return arr.Get(k), nil
		} else if po := thisVal.AsPlainObject(); po != nil {
			key := fmt.Sprintf("%d", k)
			if v, ok := po.Get(key); ok {
				return v, nil
			}
		}
		return vm.Undefined, nil
	}))

	// Array.prototype.findLast - find from end
	arrayProto.SetOwnNonEnumerable("findLast", vm.NewNativeFunction(1, false, "findLast", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.findLast called on null or undefined")
		}

		// 3. If IsCallable(predicate) is false, throw a TypeError exception.
		var predicate vm.Value
		if len(args) >= 1 {
			predicate = args[0]
		} else {
			predicate = vm.Undefined
		}
		if !predicate.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("predicate is not a function")
		}

		// Get length and iterate backwards
		if arr := thisVal.AsArray(); arr != nil {
			for i := arr.Length() - 1; i >= 0; i-- {
				element := arr.Get(i)
				result, err := vmInstance.Call(predicate, vm.Undefined, []vm.Value{element, vm.NumberValue(float64(i)), thisVal})
				if err != nil {
					return vm.Undefined, err
				}
				if result.IsTruthy() {
					return element, nil
				}
			}
		} else if po := thisVal.AsPlainObject(); po != nil {
			length := 0
			if lv, ok := po.Get("length"); ok {
				length = int(lv.ToFloat())
			}
			for i := length - 1; i >= 0; i-- {
				key := fmt.Sprintf("%d", i)
				var elem vm.Value = vm.Undefined
				if v, ok := po.Get(key); ok {
					elem = v
				}
				result, err := vmInstance.Call(predicate, vm.Undefined, []vm.Value{elem, vm.NumberValue(float64(i)), thisVal})
				if err != nil {
					return vm.Undefined, err
				}
				if result.IsTruthy() {
					return elem, nil
				}
			}
		}
		return vm.Undefined, nil
	}))

	// Array.prototype.findLastIndex - find index from end
	arrayProto.SetOwnNonEnumerable("findLastIndex", vm.NewNativeFunction(1, false, "findLastIndex", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.findLastIndex called on null or undefined")
		}

		// 3. If IsCallable(predicate) is false, throw a TypeError exception.
		var predicate vm.Value
		if len(args) >= 1 {
			predicate = args[0]
		} else {
			predicate = vm.Undefined
		}
		if !predicate.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("predicate is not a function")
		}

		// Get length and iterate backwards
		if arr := thisVal.AsArray(); arr != nil {
			for i := arr.Length() - 1; i >= 0; i-- {
				element := arr.Get(i)
				result, err := vmInstance.Call(predicate, vm.Undefined, []vm.Value{element, vm.NumberValue(float64(i)), thisVal})
				if err != nil {
					return vm.NumberValue(-1), err
				}
				if result.IsTruthy() {
					return vm.NumberValue(float64(i)), nil
				}
			}
		} else if po := thisVal.AsPlainObject(); po != nil {
			length := 0
			if lv, ok := po.Get("length"); ok {
				length = int(lv.ToFloat())
			}
			for i := length - 1; i >= 0; i-- {
				key := fmt.Sprintf("%d", i)
				var elem vm.Value = vm.Undefined
				if v, ok := po.Get(key); ok {
					elem = v
				}
				result, err := vmInstance.Call(predicate, vm.Undefined, []vm.Value{elem, vm.NumberValue(float64(i)), thisVal})
				if err != nil {
					return vm.NumberValue(-1), err
				}
				if result.IsTruthy() {
					return vm.NumberValue(float64(i)), nil
				}
			}
		}
		return vm.NumberValue(-1), nil
	}))

	// Array.prototype.copyWithin - copy sequence of elements within array
	arrayProto.SetOwnNonEnumerable("copyWithin", vm.NewNativeFunction(2, false, "copyWithin", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.copyWithin called on null or undefined")
		}

		arr := thisVal.AsArray()
		if arr == nil {
			// For array-like objects, return unchanged
			return thisVal, nil
		}

		length := arr.Length()

		// 3. Let relativeTarget be ? ToIntegerOrInfinity(target).
		// Note: Must process arguments before any early returns, as they may throw
		var target int
		if len(args) >= 1 {
			var err error
			target, err = toIntegerOrInfinityWithVM(vmInstance, args[0])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil // Let exception propagate
				}
				return vm.Undefined, err // Return TypeError
			}
		}
		// 4. If relativeTarget < 0, let to be max(len + relativeTarget, 0); else let to be min(relativeTarget, len).
		var to int
		if target < 0 {
			to = length + target
			if to < 0 {
				to = 0
			}
		} else {
			to = target
			if to > length {
				to = length
			}
		}

		// 5. Let relativeStart be ? ToIntegerOrInfinity(start).
		var start int
		if len(args) >= 2 {
			var err error
			start, err = toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil // Let exception propagate
				}
				return vm.Undefined, err // Return TypeError
			}
		}
		// 6. If relativeStart < 0, let from be max(len + relativeStart, 0); else let from be min(relativeStart, len).
		var from int
		if start < 0 {
			from = length + start
			if from < 0 {
				from = 0
			}
		} else {
			from = start
			if from > length {
				from = length
			}
		}

		// 7. If end is undefined, let relativeEnd be len; else let relativeEnd be ? ToIntegerOrInfinity(end).
		var end int
		if len(args) >= 3 && args[2].Type() != vm.TypeUndefined {
			var err error
			end, err = toIntegerOrInfinityWithVM(vmInstance, args[2])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil // Let exception propagate
				}
				return vm.Undefined, err // Return TypeError
			}
		} else {
			end = length
		}
		// 8. If relativeEnd < 0, let final be max(len + relativeEnd, 0); else let final be min(relativeEnd, len).
		var final int
		if end < 0 {
			final = length + end
			if final < 0 {
				final = 0
			}
		} else {
			final = end
			if final > length {
				final = length
			}
		}

		// 9. Let count be min(final - from, len - to).
		count := final - from
		if length-to < count {
			count = length - to
		}

		// Copy the elements
		if count > 0 {
			// Need to handle overlapping regions
			if from < to && to < from+count {
				// Copy backwards to avoid overwriting source
				for i := count - 1; i >= 0; i-- {
					arr.Set(to+i, arr.Get(from+i))
				}
			} else {
				// Copy forwards
				for i := 0; i < count; i++ {
					arr.Set(to+i, arr.Get(from+i))
				}
			}
		}

		return thisVal, nil
	}))

	// Array.prototype.fill - fill array with value
	arrayProto.SetOwnNonEnumerable("fill", vm.NewNativeFunction(1, false, "fill", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.fill called on null or undefined")
		}

		arr := thisVal.AsArray()
		if arr == nil {
			return thisVal, nil
		}

		length := arr.Length()

		// Get value to fill
		value := vm.Undefined
		if len(args) >= 1 {
			value = args[0]
		}

		// Get start index (? ToIntegerOrInfinity)
		var start int
		if len(args) >= 2 {
			var err error
			start, err = toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
		}
		if start < 0 {
			start = length + start
			if start < 0 {
				start = 0
			}
		} else if start > length {
			start = length
		}

		// Get end index (? ToIntegerOrInfinity)
		end := length
		if len(args) >= 3 && args[2].Type() != vm.TypeUndefined {
			var err error
			end, err = toIntegerOrInfinityWithVM(vmInstance, args[2])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
		}
		if end < 0 {
			end = length + end
			if end < 0 {
				end = 0
			}
		} else if end > length {
			end = length
		}

		// Fill the array
		for i := start; i < end; i++ {
			arr.Set(i, value)
		}

		return thisVal, nil
	}))

	// Array.prototype.flat - flatten nested arrays
	arrayProto.SetOwnNonEnumerable("flat", vm.NewNativeFunction(0, false, "flat", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.flat called on null or undefined")
		}

		// 3. Let depthNum be ? ToIntegerOrInfinity(depth) (default 1)
		depth := 1
		if len(args) >= 1 && args[0].Type() != vm.TypeUndefined {
			var err error
			depth, err = toIntegerOrInfinityWithVM(vmInstance, args[0])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
		}

		result := vm.NewArray()
		resultArr := result.AsArray()

		// Helper function to flatten recursively
		var flattenInto func(source vm.Value, currentDepth int)
		flattenInto = func(source vm.Value, currentDepth int) {
			var length int
			var getElement func(i int) vm.Value

			if arr := source.AsArray(); arr != nil {
				length = arr.Length()
				getElement = func(i int) vm.Value { return arr.Get(i) }
			} else if po := source.AsPlainObject(); po != nil {
				if lv, ok := po.Get("length"); ok {
					length = int(lv.ToFloat())
				}
				getElement = func(i int) vm.Value {
					if v, ok := po.Get(fmt.Sprintf("%d", i)); ok {
						return v
					}
					return vm.Undefined
				}
			} else {
				return
			}

			for i := 0; i < length; i++ {
				element := getElement(i)
				if currentDepth > 0 && element.IsArray() {
					flattenInto(element, currentDepth-1)
				} else {
					resultArr.Append(element)
				}
			}
		}

		flattenInto(thisVal, depth)
		return result, nil
	}))

	// Array.prototype.flatMap - map then flatten by one level
	arrayProto.SetOwnNonEnumerable("flatMap", vm.NewNativeFunction(1, false, "flatMap", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.flatMap called on null or undefined")
		}

		// 3. If IsCallable(mapperFunction) is false, throw a TypeError exception.
		var mapper vm.Value
		if len(args) >= 1 {
			mapper = args[0]
		} else {
			mapper = vm.Undefined
		}
		if !mapper.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("flatMap mapper is not a function")
		}

		result := vm.NewArray()
		resultArr := result.AsArray()

		// Get length and iterate
		if arr := thisVal.AsArray(); arr != nil {
			for i := 0; i < arr.Length(); i++ {
				element := arr.Get(i)
				mapped, err := vmInstance.Call(mapper, vm.Undefined, []vm.Value{element, vm.NumberValue(float64(i)), thisVal})
				if err != nil {
					return vm.Undefined, err
				}
				// Flatten by one level
				if mappedArr := mapped.AsArray(); mappedArr != nil {
					for j := 0; j < mappedArr.Length(); j++ {
						resultArr.Append(mappedArr.Get(j))
					}
				} else {
					resultArr.Append(mapped)
				}
			}
		} else if po := thisVal.AsPlainObject(); po != nil {
			length := 0
			if lv, ok := po.Get("length"); ok {
				length = int(lv.ToFloat())
			}
			for i := 0; i < length; i++ {
				key := fmt.Sprintf("%d", i)
				var elem vm.Value = vm.Undefined
				if v, ok := po.Get(key); ok {
					elem = v
				}
				mapped, err := vmInstance.Call(mapper, vm.Undefined, []vm.Value{elem, vm.NumberValue(float64(i)), thisVal})
				if err != nil {
					return vm.Undefined, err
				}
				if mappedArr := mapped.AsArray(); mappedArr != nil {
					for j := 0; j < mappedArr.Length(); j++ {
						resultArr.Append(mappedArr.Get(j))
					}
				} else {
					resultArr.Append(mapped)
				}
			}
		}

		return result, nil
	}))

	// Array.prototype.toSorted - non-mutating sort
	arrayProto.SetOwnNonEnumerable("toSorted", vm.NewNativeFunction(1, false, "toSorted", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.toSorted called on null or undefined")
		}

		// Check comparator if provided
		var comparator vm.Value
		if len(args) >= 1 && args[0].Type() != vm.TypeUndefined {
			comparator = args[0]
			if !comparator.IsCallable() {
				return vm.Undefined, vmInstance.NewTypeError("comparator is not a function")
			}
		}

		// Create a copy
		result := vm.NewArray()
		resultArr := result.AsArray()

		if arr := thisVal.AsArray(); arr != nil {
			for i := 0; i < arr.Length(); i++ {
				resultArr.Append(arr.Get(i))
			}
		} else if po := thisVal.AsPlainObject(); po != nil {
			length := 0
			if lv, ok := po.Get("length"); ok {
				length = int(lv.ToFloat())
			}
			for i := 0; i < length; i++ {
				key := fmt.Sprintf("%d", i)
				if v, ok := po.Get(key); ok {
					resultArr.Append(v)
				} else {
					resultArr.Append(vm.Undefined)
				}
			}
		}

		// Sort the copy using the same logic as sort
		n := resultArr.Length()
		if n <= 1 {
			return result, nil
		}

		// Simple insertion sort for stability
		for i := 1; i < n; i++ {
			key := resultArr.Get(i)
			j := i - 1
			for j >= 0 {
				cmp := 0
				if comparator.IsCallable() {
					res, err := vmInstance.Call(comparator, vm.Undefined, []vm.Value{resultArr.Get(j), key})
					if err != nil {
						return vm.Undefined, err
					}
					cmp = int(res.ToFloat())
				} else {
					// Default string comparison
					a := resultArr.Get(j).ToString()
					b := key.ToString()
					if a > b {
						cmp = 1
					} else if a < b {
						cmp = -1
					}
				}
				if cmp <= 0 {
					break
				}
				resultArr.Set(j+1, resultArr.Get(j))
				j--
			}
			resultArr.Set(j+1, key)
		}

		return result, nil
	}))

	// Array.prototype.toSpliced - non-mutating splice
	arrayProto.SetOwnNonEnumerable("toSpliced", vm.NewNativeFunction(2, true, "toSpliced", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.toSpliced called on null or undefined")
		}

		// Get source length
		var sourceLength int
		if arr := thisVal.AsArray(); arr != nil {
			sourceLength = arr.Length()
		} else if po := thisVal.AsPlainObject(); po != nil {
			if lv, ok := po.Get("length"); ok {
				sourceLength = int(lv.ToFloat())
			}
		}

		// Parse start argument (? ToIntegerOrInfinity)
		var actualStart int
		if len(args) >= 1 {
			start, err := toIntegerOrInfinityWithVM(vmInstance, args[0])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
			if start < 0 {
				actualStart = sourceLength + start
				if actualStart < 0 {
					actualStart = 0
				}
			} else {
				actualStart = start
				if actualStart > sourceLength {
					actualStart = sourceLength
				}
			}
		}

		// Parse deleteCount argument (? ToIntegerOrInfinity)
		actualDeleteCount := 0
		if len(args) >= 2 {
			deleteCount, err := toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
			if deleteCount < 0 {
				deleteCount = 0
			}
			actualDeleteCount = deleteCount
			if actualDeleteCount > sourceLength-actualStart {
				actualDeleteCount = sourceLength - actualStart
			}
		} else if len(args) >= 1 {
			// If start is present but deleteCount is not, delete to end
			actualDeleteCount = sourceLength - actualStart
		}

		// Items to insert
		insertItems := []vm.Value{}
		if len(args) > 2 {
			insertItems = args[2:]
		}

		// Create result array
		result := vm.NewArray()
		resultArr := result.AsArray()

		// Helper to get element
		getElement := func(i int) vm.Value {
			if arr := thisVal.AsArray(); arr != nil {
				return arr.Get(i)
			} else if po := thisVal.AsPlainObject(); po != nil {
				if v, ok := po.Get(fmt.Sprintf("%d", i)); ok {
					return v
				}
			}
			return vm.Undefined
		}

		// Copy elements before start
		for i := 0; i < actualStart; i++ {
			resultArr.Append(getElement(i))
		}

		// Insert new elements
		for _, item := range insertItems {
			resultArr.Append(item)
		}

		// Copy remaining elements after deleted section
		for i := actualStart + actualDeleteCount; i < sourceLength; i++ {
			resultArr.Append(getElement(i))
		}

		return result, nil
	}))

	// Array.prototype.with - non-mutating element replacement
	arrayProto.SetOwnNonEnumerable("with", vm.NewNativeFunction(2, false, "with", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.with called on null or undefined")
		}

		// Get length
		var length int
		if arr := thisVal.AsArray(); arr != nil {
			length = arr.Length()
		} else if po := thisVal.AsPlainObject(); po != nil {
			if lv, ok := po.Get("length"); ok {
				length = int(lv.ToFloat())
			}
		}

		// 3. Let relativeIndex be ? ToIntegerOrInfinity(index).
		var index int
		if len(args) >= 1 {
			var err error
			index, err = toIntegerOrInfinityWithVM(vmInstance, args[0])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
		}

		// 4. If relativeIndex ≥ 0, let actualIndex be relativeIndex. Else let actualIndex be len + relativeIndex.
		actualIndex := index
		if index < 0 {
			actualIndex = length + index
		}

		// 5. If actualIndex ≥ len or actualIndex < 0, throw a RangeError exception.
		if actualIndex < 0 || actualIndex >= length {
			return vm.Undefined, vmInstance.NewRangeError(fmt.Sprintf("Invalid index: %d", index))
		}

		// Get value
		value := vm.Undefined
		if len(args) >= 2 {
			value = args[1]
		}

		// Create copy with replaced element
		result := vm.NewArray()
		resultArr := result.AsArray()

		if arr := thisVal.AsArray(); arr != nil {
			for i := 0; i < length; i++ {
				if i == actualIndex {
					resultArr.Append(value)
				} else {
					resultArr.Append(arr.Get(i))
				}
			}
		} else if po := thisVal.AsPlainObject(); po != nil {
			for i := 0; i < length; i++ {
				if i == actualIndex {
					resultArr.Append(value)
				} else {
					key := fmt.Sprintf("%d", i)
					if v, ok := po.Get(key); ok {
						resultArr.Append(v)
					} else {
						resultArr.Append(vm.Undefined)
					}
				}
			}
		}

		return result, nil
	}))

	// Array.prototype.toReversed - non-mutating reverse
	arrayProto.SetOwnNonEnumerable("toReversed", vm.NewNativeFunction(0, false, "toReversed", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.toReversed called on null or undefined")
		}

		// Get length
		var length int
		if arr := thisVal.AsArray(); arr != nil {
			length = arr.Length()
		} else if po := thisVal.AsPlainObject(); po != nil {
			if lv, ok := po.Get("length"); ok {
				length = int(lv.ToFloat())
			}
		}

		// Create reversed copy
		result := vm.NewArray()
		resultArr := result.AsArray()

		if arr := thisVal.AsArray(); arr != nil {
			for i := length - 1; i >= 0; i-- {
				resultArr.Append(arr.Get(i))
			}
		} else if po := thisVal.AsPlainObject(); po != nil {
			for i := length - 1; i >= 0; i-- {
				key := fmt.Sprintf("%d", i)
				if v, ok := po.Get(key); ok {
					resultArr.Append(v)
				} else {
					resultArr.Append(vm.Undefined)
				}
			}
		}

		return result, nil
	}))

	// Create Array constructor (length=1 per spec, variadic for multiple args)
	ctorWithProps := vm.NewConstructorWithProps(1, true, "Array", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewArray(), nil
		}
		if len(args) == 1 {
			// If single argument is a number, create array with that length
			if args[0].IsNumber() {
				length := int(args[0].ToFloat())
				if length < 0 {
					return vm.NewArray(), nil // Should throw RangeError in real JS
				}
				result := vm.NewArray()
				// Set length without allocating elements - JavaScript arrays are sparse
				arr := result.AsArray()
				arr.SetLength(length)
				// DEBUG: verify length was set
				// fmt.Printf("[DEBUG Array constructor] Set length to %d, arr.Length() = %d, result.Type() = %d (TypeArray=%d)\n", length, arr.Length(), result.Type(), vm.TypeArray)
				return result, nil
			}
		}
		// Multiple arguments or single non-number argument - create array with those elements
		return vm.NewArrayWithArgs(args), nil
	})

	// Add prototype property
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(arrayProto))

	// Add static methods
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("isArray", vm.NewNativeFunction(1, false, "isArray", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}
		arg := args[0]
		// Check if it's an Array type
		if arg.Type() == vm.TypeArray {
			return vm.BooleanValue(true), nil
		}
		// Per ECMAScript, Array.prototype is an Array exotic object and isArray should return true
		// Check if arg is the Array.prototype object
		if arg.IsObject() && vmInstance.ArrayPrototype.IsObject() {
			if arg.AsPlainObject() == vmInstance.ArrayPrototype.AsPlainObject() {
				return vm.BooleanValue(true), nil
			}
		}
		return vm.BooleanValue(false), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.NewArray(), nil
		}
		arrayLike := args[0]

		// Get optional mapFn and thisArg
		var mapFn vm.Value = vm.Undefined
		var thisArg vm.Value = vm.Undefined
		if len(args) >= 2 && args[1].IsCallable() {
			mapFn = args[1]
		}
		if len(args) >= 3 {
			thisArg = args[2]
		}

		// If it's already an array, create a shallow copy
		if arrayLike.Type() == vm.TypeArray {
			sourceArray := arrayLike.AsArray()
			result := vm.NewArray()
			for i := 0; i < sourceArray.Length(); i++ {
				element := sourceArray.Get(i)
				// Apply mapping function if provided
				if mapFn.Type() != vm.TypeUndefined {
					vmInstance.EnterHelperCall()
					mapped, err := vmInstance.Call(mapFn, thisArg, []vm.Value{element, vm.NumberValue(float64(i))})
					vmInstance.ExitHelperCall()
					if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
						return vm.NewArray(), nil
					}
					if err != nil {
						return vm.NewArray(), err
					}
					result.AsArray().Append(mapped)
				} else {
					result.AsArray().Append(element)
				}
			}
			return result, nil
		}

		// Check if the source is iterable (has Symbol.iterator)
		iteratorMethod := vm.Undefined
		hasIterator := false
		if arrayLike.Type() == vm.TypeSet || arrayLike.Type() == vm.TypeMap {
			if method, ok := vmInstance.GetSymbolProperty(arrayLike, SymbolIterator); ok && method.IsCallable() {
				iteratorMethod = method
				hasIterator = true
			}
		} else if arrayLike.IsObject() {
			if method, ok := vmInstance.GetSymbolProperty(arrayLike, SymbolIterator); ok && method.IsCallable() {
				iteratorMethod = method
				hasIterator = true
			}
		}

		if hasIterator {
			// Use iterator protocol
			result := vm.NewArray()
			vmInstance.EnterHelperCall()
			iterator, err := vmInstance.Call(iteratorMethod, arrayLike, []vm.Value{})
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.NewArray(), nil
			}
			if err != nil {
				return vm.NewArray(), err
			}

			nextMethod, err := vmInstance.GetProperty(iterator, "next")
			if err != nil || !nextMethod.IsCallable() {
				return vm.NewArray(), vmInstance.NewTypeError("iterator.next is not a function")
			}

			index := 0
			for {
				vmInstance.EnterHelperCall()
				iterResult, err := vmInstance.Call(nextMethod, iterator, []vm.Value{})
				vmInstance.ExitHelperCall()
				if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
					return vm.NewArray(), nil
				}
				if err != nil {
					return vm.NewArray(), err
				}

				done, _ := vmInstance.GetProperty(iterResult, "done")
				if done.IsTruthy() {
					break
				}

				value, _ := vmInstance.GetProperty(iterResult, "value")
				// Apply mapping function if provided
				if mapFn.Type() != vm.TypeUndefined {
					vmInstance.EnterHelperCall()
					mapped, mapErr := vmInstance.Call(mapFn, thisArg, []vm.Value{value, vm.NumberValue(float64(index))})
					vmInstance.ExitHelperCall()
					if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
						return vm.NewArray(), nil
					}
					if mapErr != nil {
						return vm.NewArray(), mapErr
					}
					result.AsArray().Append(mapped)
				} else {
					result.AsArray().Append(value)
				}
				index++
			}
			return result, nil
		}

		// For array-like objects (has length property)
		if arrayLike.IsObject() {
			lengthVal, err := vmInstance.GetProperty(arrayLike, "length")
			if err == nil && lengthVal.IsNumber() {
				length := int(lengthVal.ToFloat())
				result := vm.NewArray()
				for i := 0; i < length; i++ {
					element, _ := vmInstance.GetProperty(arrayLike, fmt.Sprintf("%d", i))
					// Apply mapping function if provided
					if mapFn.Type() != vm.TypeUndefined {
						vmInstance.EnterHelperCall()
						mapped, mapErr := vmInstance.Call(mapFn, thisArg, []vm.Value{element, vm.NumberValue(float64(i))})
						vmInstance.ExitHelperCall()
						if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
							return vm.NewArray(), nil
						}
						if mapErr != nil {
							return vm.NewArray(), mapErr
						}
						result.AsArray().Append(mapped)
					} else {
						result.AsArray().Append(element)
					}
				}
				return result, nil
			}
		}

		// Fallback: return empty array
		return vm.NewArray(), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		// Array.of ( ...items )
		// 1. Let len be the actual number of arguments passed to this function.
		len := len(args)

		// 3. Let C be the this value.
		C := vmInstance.GetThis()

		var A vm.Value

		// 4. If IsConstructor(C) is true, then
		//    a. Let A be ? Construct(C, « len »).
		if C.IsCallable() && vmInstance.IsConstructor(C) {
			// Call C as constructor with len as single argument
			result, err := vmInstance.Construct(C, []vm.Value{vm.NumberValue(float64(len))})
			if err != nil {
				return vm.Undefined, err
			}
			A = result
		} else {
			// 5. Else,
			//    a. Let A be ? ArrayCreate(len).
			A = vm.NewArrayWithLength(len)
		}

		// 6. Let k be 0.
		// 7. Repeat, while k < len
		for k := 0; k < len; k++ {
			// a. Let kValue be items[k].
			kValue := args[k]
			// b. Let Pk be ! ToString(k).
			pk := fmt.Sprintf("%d", k)
			// c. Perform ? CreateDataPropertyOrThrow(A, Pk, kValue).
			if A.IsArray() {
				arr := A.AsArray()
				arr.Set(k, kValue)
			} else if A.IsObject() {
				po := A.AsPlainObject()
				// Check if we can create/update this property
				// CreateDataPropertyOrThrow fails if:
				// 1. Object is non-extensible and property doesn't exist
				// 2. Property exists but is non-configurable
				// GetOwnDescriptor returns: (Value, writable, enumerable, configurable, exists)
				_, _, _, configurable, existingProp := po.GetOwnDescriptor(pk)
				if !existingProp && !po.IsExtensible() {
					return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("Cannot add property %s, object is not extensible", pk))
				}
				if existingProp && !configurable {
					return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("Cannot redefine property: %s", pk))
				}
				w, e, c := true, true, true // writable, enumerable, configurable
				po.DefineOwnProperty(pk, kValue, &w, &e, &c)
			}
		}

		// 8. Perform ? Set(A, "length", len, true).
		if A.IsArray() {
			// Already set by Set
		} else if A.IsObject() {
			po := A.AsPlainObject()
			lengthVal := vm.NumberValue(float64(len))
			// Check if there's a setter for "length" and call it
			if _, setter, _, _, ok := po.GetOwnAccessor("length"); ok && setter.Type() != vm.TypeUndefined {
				_, err := vmInstance.Call(setter, A, []vm.Value{lengthVal})
				if err != nil {
					return vm.Undefined, err
				}
			} else {
				po.SetOwn("length", lengthVal)
			}
		}

		// 9. Return A.
		return A, nil
	}))

	// Array.fromAsync ( asyncItems [ , mapfn [ , thisArg ] ] )
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("fromAsync", vm.NewNativeFunction(1, false, "fromAsync", func(args []vm.Value) (vm.Value, error) {
		// Get asyncItems
		var asyncItems vm.Value = vm.Undefined
		if len(args) > 0 {
			asyncItems = args[0]
		}

		// Check if asyncItems is undefined or null
		if asyncItems.Type() == vm.TypeUndefined || asyncItems.Type() == vm.TypeNull {
			// Return a rejected promise with TypeError
			reason := vmInstance.NewTypeError("Cannot convert undefined or null to object")
			return vmInstance.NewRejectedPromise(vm.NewString(reason.Error())), nil
		}

		// Get mapfn (optional)
		var mapfn vm.Value = vm.Undefined
		if len(args) > 1 {
			mapfn = args[1]
		}

		// Get thisArg (optional)
		var thisArg vm.Value = vm.Undefined
		if len(args) > 2 {
			thisArg = args[2]
		}

		// If mapfn is not undefined and not callable, throw TypeError
		if mapfn.Type() != vm.TypeUndefined && !mapfn.IsCallable() {
			reason := vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", mapfn.Type().String()))
			return vmInstance.NewRejectedPromise(vm.NewString(reason.Error())), nil
		}

		// Get C (the this value - for subclass support)
		C := vmInstance.GetThis()

		// Create the promise that will be returned
		// We need to set up async iteration or array-like processing
		// This is done inside a promise executor

		// Create pending promise and resolve/reject functions
		promise := vmInstance.NewPendingPromise()
		promiseObj := promise.AsPromise()

		// Helper function to resolve the promise with the result array
		resolveWithArray := func(arr vm.Value) {
			vmInstance.ResolvePromise(promiseObj, arr)
		}

		// Helper function to reject the promise
		rejectWithError := func(err error) {
			vmInstance.RejectPromise(promiseObj, vm.NewString(err.Error()))
		}

		// Start async processing
		rt := vmInstance.GetAsyncRuntime()
		rt.ScheduleMicrotask(func() {
			// Try to get async iterator first
			var iteratorMethod vm.Value = vm.Undefined
			var usingAsyncIterator bool = false

			// Check for Symbol.asyncIterator using the VM's GetSymbolProperty
			if method, ok := vmInstance.GetSymbolProperty(asyncItems, SymbolAsyncIterator); ok && method.IsCallable() {
				iteratorMethod = method
				usingAsyncIterator = true
			}

			// If no async iterator, check for sync iterator
			if iteratorMethod.Type() == vm.TypeUndefined || !iteratorMethod.IsCallable() {
				usingAsyncIterator = false
				if method, ok := vmInstance.GetSymbolProperty(asyncItems, SymbolIterator); ok && method.IsCallable() {
					iteratorMethod = method
				}
			}

			// Create result array using constructor if appropriate
			var A vm.Value
			if C.IsCallable() && vmInstance.IsConstructor(C) {
				// Use the constructor
				result, err := vmInstance.Construct(C, []vm.Value{vm.NumberValue(0)})
				if err != nil {
					rejectWithError(err)
					return
				}
				A = result
			} else {
				A = vm.NewArray()
			}

			// Process using iterator or array-like
			if iteratorMethod.IsCallable() {
				// Use iterator
				iterator, err := vmInstance.Call(iteratorMethod, asyncItems, []vm.Value{})
				if err != nil {
					rejectWithError(err)
					return
				}

				// Get next method - handle different iterator types
				var nextMethod vm.Value = vm.Undefined

				// For plain objects, use GetProperty
				if iterator.Type() == vm.TypeObject {
					nextMethod, err = vmInstance.GetProperty(iterator, "next")
					if err != nil {
						rejectWithError(err)
						return
					}
				} else if iterator.Type() == vm.TypeGenerator {
					// For generators, get from GeneratorPrototype
					if vmInstance.GeneratorPrototype.Type() != vm.TypeUndefined {
						proto := vmInstance.GeneratorPrototype.AsPlainObject()
						if proto != nil {
							if m, ok := proto.Get("next"); ok {
								nextMethod = m
							}
						}
					}
				} else if iterator.Type() == vm.TypeAsyncGenerator {
					// For async generators, get from AsyncGeneratorPrototype
					if vmInstance.AsyncGeneratorPrototype.Type() != vm.TypeUndefined {
						proto := vmInstance.AsyncGeneratorPrototype.AsPlainObject()
						if proto != nil {
							if m, ok := proto.Get("next"); ok {
								nextMethod = m
							}
						}
					}
				}

				if !nextMethod.IsCallable() {
					rejectWithError(errors.New("iterator.next is not a function"))
					return
				}

				// Iterate asynchronously
				k := 0
				var iterateNext func()
				iterateNext = func() {
					// Call next()
					nextResult, err := vmInstance.Call(nextMethod, iterator, []vm.Value{})
					if err != nil {
						rejectWithError(err)
						return
					}

					// Handle async iterator (nextResult might be a promise)
					var handleNextResult func(result vm.Value)
					handleNextResult = func(result vm.Value) {
						// Get done and value
						var done bool = false
						var value vm.Value = vm.Undefined

						if result.IsObject() {
							obj := result.AsPlainObject()
							if obj != nil {
								if doneVal, ok := obj.GetOwn("done"); ok {
									done = doneVal.IsTruthy()
								}
								if v, ok := obj.GetOwn("value"); ok {
									value = v
								}
							}
						}

						if done {
							// Set length and resolve
							if A.IsArray() {
								// Length is automatically updated for arrays
							} else if A.IsObject() {
								A.AsPlainObject().SetOwn("length", vm.NumberValue(float64(k)))
							}
							resolveWithArray(A)
							return
						}

						// For async iterator, the value might be a promise
						var handleValue func(val vm.Value)
						handleValue = func(val vm.Value) {
							// Apply mapfn if present
							var mappedValue vm.Value = val
							if mapfn.IsCallable() {
								result, err := vmInstance.Call(mapfn, thisArg, []vm.Value{val, vm.NumberValue(float64(k))})
								if err != nil {
									rejectWithError(err)
									return
								}
								mappedValue = result
							}

							// If mappedValue is a promise, wait for it
							if mappedValue.Type() == vm.TypePromise {
								mp := mappedValue.AsPromise()
								if mp != nil && mp.State == vm.PromisePending {
									vmInstance.AddPromiseReaction(mappedValue, true, func(v vm.Value) {
										// Add to result array
										if A.IsArray() {
											A.AsArray().Set(k, v)
										} else if A.IsObject() {
											A.AsPlainObject().SetOwn(fmt.Sprintf("%d", k), v)
										}
										k++
										iterateNext()
									})
									vmInstance.AddPromiseReaction(mappedValue, false, func(r vm.Value) {
										rejectWithError(errors.New(r.ToString()))
									})
									return
								} else if mp != nil && mp.State == vm.PromiseFulfilled {
									mappedValue = mp.Result
								} else if mp != nil && mp.State == vm.PromiseRejected {
									rejectWithError(errors.New(mp.Result.ToString()))
									return
								}
							}

							// Add to result array
							if A.IsArray() {
								A.AsArray().Set(k, mappedValue)
							} else if A.IsObject() {
								A.AsPlainObject().SetOwn(fmt.Sprintf("%d", k), mappedValue)
							}
							k++
							rt.ScheduleMicrotask(iterateNext)
						}

						// If value is a promise (for sync iterator with promise elements), wait for it
						if value.Type() == vm.TypePromise {
							vp := value.AsPromise()
							if vp != nil && vp.State == vm.PromisePending {
								vmInstance.AddPromiseReaction(value, true, handleValue)
								vmInstance.AddPromiseReaction(value, false, func(r vm.Value) {
									rejectWithError(errors.New(r.ToString()))
								})
								return
							} else if vp != nil && vp.State == vm.PromiseFulfilled {
								value = vp.Result
							} else if vp != nil && vp.State == vm.PromiseRejected {
								rejectWithError(errors.New(vp.Result.ToString()))
								return
							}
						}
						handleValue(value)
					}

					// If nextResult is a promise (async iterator), wait for it
					if usingAsyncIterator && nextResult.Type() == vm.TypePromise {
						np := nextResult.AsPromise()
						if np != nil && np.State == vm.PromisePending {
							vmInstance.AddPromiseReaction(nextResult, true, handleNextResult)
							vmInstance.AddPromiseReaction(nextResult, false, func(r vm.Value) {
								rejectWithError(errors.New(r.ToString()))
							})
							return
						} else if np != nil && np.State == vm.PromiseFulfilled {
							nextResult = np.Result
						} else if np != nil && np.State == vm.PromiseRejected {
							rejectWithError(errors.New(np.Result.ToString()))
							return
						}
					}
					handleNextResult(nextResult)
				}

				iterateNext()
			} else {
				// Array-like: use length property
				var length int = 0

				if asyncItems.IsArray() {
					length = asyncItems.AsArray().Length()
				} else if asyncItems.IsObject() {
					obj := asyncItems.AsPlainObject()
					if obj != nil {
						if lenVal, ok := obj.GetOwn("length"); ok {
							length = int(lenVal.ToFloat())
						}
					}
				} else {
					// For primitives, treat as empty
					resolveWithArray(A)
					return
				}

				if length == 0 {
					resolveWithArray(A)
					return
				}

				// Process array-like elements
				k := 0
				var processNext func()
				processNext = func() {
					if k >= length {
						// Set length and resolve
						if A.IsArray() {
							// Length is auto-updated
						} else if A.IsObject() {
							A.AsPlainObject().SetOwn("length", vm.NumberValue(float64(k)))
						}
						resolveWithArray(A)
						return
					}

					// Get element at k
					var kValue vm.Value = vm.Undefined
					if asyncItems.IsArray() {
						kValue = asyncItems.AsArray().Get(k)
					} else if asyncItems.IsObject() {
						obj := asyncItems.AsPlainObject()
						if obj != nil {
							if v, ok := obj.GetOwn(fmt.Sprintf("%d", k)); ok {
								kValue = v
							}
						}
					}

					// Handle the value (might be a promise)
					var handleKValue func(val vm.Value)
					handleKValue = func(val vm.Value) {
						// Apply mapfn if present
						var mappedValue vm.Value = val
						if mapfn.IsCallable() {
							result, err := vmInstance.Call(mapfn, thisArg, []vm.Value{val, vm.NumberValue(float64(k))})
							if err != nil {
								rejectWithError(err)
								return
							}
							mappedValue = result
						}

						// If mappedValue is a promise, wait for it
						if mappedValue.Type() == vm.TypePromise {
							mp := mappedValue.AsPromise()
							if mp != nil && mp.State == vm.PromisePending {
								vmInstance.AddPromiseReaction(mappedValue, true, func(v vm.Value) {
									// Add to result array
									if A.IsArray() {
										A.AsArray().Set(k, v)
									} else if A.IsObject() {
										A.AsPlainObject().SetOwn(fmt.Sprintf("%d", k), v)
									}
									k++
									processNext()
								})
								vmInstance.AddPromiseReaction(mappedValue, false, func(r vm.Value) {
									rejectWithError(errors.New(r.ToString()))
								})
								return
							} else if mp != nil && mp.State == vm.PromiseFulfilled {
								mappedValue = mp.Result
							} else if mp != nil && mp.State == vm.PromiseRejected {
								rejectWithError(errors.New(mp.Result.ToString()))
								return
							}
						}

						// Add to result array
						if A.IsArray() {
							A.AsArray().Set(k, mappedValue)
						} else if A.IsObject() {
							A.AsPlainObject().SetOwn(fmt.Sprintf("%d", k), mappedValue)
						}
						k++
						rt.ScheduleMicrotask(processNext)
					}

					// If kValue is a promise, wait for it
					if kValue.Type() == vm.TypePromise {
						kp := kValue.AsPromise()
						if kp != nil && kp.State == vm.PromisePending {
							vmInstance.AddPromiseReaction(kValue, true, handleKValue)
							vmInstance.AddPromiseReaction(kValue, false, func(r vm.Value) {
								rejectWithError(errors.New(r.ToString()))
							})
							return
						} else if kp != nil && kp.State == vm.PromiseFulfilled {
							kValue = kp.Result
						} else if kp != nil && kp.State == vm.PromiseRejected {
							rejectWithError(errors.New(kp.Result.ToString()))
							return
						}
					}
					handleKValue(kValue)
				}

				processNext()
			}
		})

		return promise, nil
	}))

	// Add Symbol.iterator implementation for arrays and array-like objects (Arguments)
	// Use the global SymbolIterator (Symbol initializes before Array now)
	// Register [Symbol.iterator] using native symbol key
	iterFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()

		// First check for array type (AsArray() panics on wrong type, so check type first)
		if thisVal.Type() == vm.TypeArray {
			return createArrayIterator(vmInstance, thisVal.AsArray()), nil
		}

		// Handle Arguments objects (array-like)
		if thisVal.Type() == vm.TypeArguments {
			argsObj := thisVal.AsArguments()
			return createArgumentsIterator(vmInstance, argsObj), nil
		}

		// Handle generic array-like objects with length property
		if thisVal.IsObject() {
			obj := thisVal.AsPlainObject()
			if obj != nil {
				if lenVal, ok := obj.GetOwn("length"); ok && lenVal.IsNumber() {
					return createArrayLikeIterator(vmInstance, thisVal), nil
				}
			}
		}

		return vm.Undefined, nil
	})
	// Native symbol key - make it writable and configurable like standard JavaScript
	w, e, c := true, false, true // writable, not enumerable, configurable
	arrayProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), iterFn, &w, &e, &c)

	// Array.prototype.values() - returns iterator yielding values (same as [Symbol.iterator])
	valuesFn := vm.NewNativeFunction(0, false, "values", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeArray {
			return createArrayIterator(vmInstance, thisVal.AsArray()), nil
		}
		// Handle array-like objects
		if thisVal.IsObject() {
			return createArrayLikeIterator(vmInstance, thisVal), nil
		}
		return vm.Undefined, nil
	})
	arrayProto.SetOwnNonEnumerable("values", valuesFn)

	// Array.prototype.keys() - returns iterator yielding indices
	keysFn := vm.NewNativeFunction(0, false, "keys", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		return createArrayKeysIterator(vmInstance, thisVal), nil
	})
	arrayProto.SetOwnNonEnumerable("keys", keysFn)

	// Array.prototype.entries() - returns iterator yielding [index, value] pairs
	entriesFn := vm.NewNativeFunction(0, false, "entries", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		return createArrayEntriesIterator(vmInstance, thisVal), nil
	})
	arrayProto.SetOwnNonEnumerable("entries", entriesFn)

	// Add Symbol.asyncIterator implementation for arrays (for await...of support)
	// This wraps the sync iterator in an async iterator (returns promises)
	asyncIterFn := vm.NewNativeFunction(0, false, "[Symbol.asyncIterator]", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis().AsArray()
		if thisArray == nil {
			return vm.Undefined, nil
		}

		// Create an async array iterator object (wraps sync iterator)
		return createAsyncArrayIterator(vmInstance, thisArray), nil
	})
	// Make it writable and configurable like standard JavaScript
	w2, e2, c2 := true, false, true
	arrayProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolAsyncIterator), asyncIterFn, &w2, &e2, &c2)

	arrayCtor := ctorWithProps

	// Set constructor property on Array.prototype to point to Array constructor
	arrayProto.SetOwnNonEnumerable("constructor", arrayCtor)
	// Make it non-enumerable like in standard JavaScript
	if v, ok := arrayProto.GetOwn("constructor"); ok {
		w, e, c := true, false, true // writable, not enumerable, configurable
		arrayProto.DefineOwnProperty("constructor", v, &w, &e, &c)
	}
	// Set Array prototype in VM
	vmInstance.ArrayPrototype = vm.NewValueFromPlainObject(arrayProto)

	// Register Array constructor as global
	return ctx.DefineGlobal("Array", arrayCtor)
}

// createArrayIterator creates an iterator object for array iteration
func createArrayIterator(vmInstance *vm.VM, array *vm.ArrayObject) vm.Value {
	// Create iterator object inheriting from Object.prototype
	iterator := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	iteratorVal := vm.NewValueFromPlainObject(iterator)

	// Iterator state: current index
	currentIndex := 0

	// Add next() method to iterator
	iterator.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		// Create iterator result object {value, done}
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

		if currentIndex >= array.Length() {
			// Iterator is exhausted
			result.SetOwnNonEnumerable("value", vm.Undefined)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(true))
		} else {
			// Return current element and advance
			val := array.Get(currentIndex)
			result.SetOwnNonEnumerable("value", val)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(false))
			currentIndex++
		}

		return vm.NewValueFromPlainObject(result), nil
	}))

	// Add [Symbol.iterator] that returns the iterator itself (required for for-of)
	iterSelfFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(args []vm.Value) (vm.Value, error) {
		return iteratorVal, nil
	})
	w, e, c := true, false, true
	iterator.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), iterSelfFn, &w, &e, &c)

	return iteratorVal
}

// createArgumentsIterator creates an iterator object for Arguments objects
func createArgumentsIterator(vmInstance *vm.VM, args *vm.ArgumentsObject) vm.Value {
	// Create iterator object inheriting from Object.prototype
	iterator := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	iteratorVal := vm.NewValueFromPlainObject(iterator)

	// Iterator state: current index
	currentIndex := 0

	// Add next() method to iterator
	iterator.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(innerArgs []vm.Value) (vm.Value, error) {
		// Create iterator result object {value, done}
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

		if currentIndex >= args.Length() {
			// Iterator is exhausted
			result.SetOwnNonEnumerable("value", vm.Undefined)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(true))
		} else {
			// Return current element and advance
			val := args.Get(currentIndex)
			result.SetOwnNonEnumerable("value", val)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(false))
			currentIndex++
		}

		return vm.NewValueFromPlainObject(result), nil
	}))

	// Add [Symbol.iterator] that returns the iterator itself
	iterSelfFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(fnArgs []vm.Value) (vm.Value, error) {
		return iteratorVal, nil
	})
	w, e, c := true, false, true
	iterator.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), iterSelfFn, &w, &e, &c)

	return iteratorVal
}

// createArrayLikeIterator creates an iterator for generic array-like objects (with length and indices)
func createArrayLikeIterator(vmInstance *vm.VM, arrayLike vm.Value) vm.Value {
	// Create iterator object inheriting from Object.prototype
	iterator := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	iteratorVal := vm.NewValueFromPlainObject(iterator)

	// Iterator state: current index
	currentIndex := 0

	// Add next() method to iterator
	iterator.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(innerArgs []vm.Value) (vm.Value, error) {
		// Create iterator result object {value, done}
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

		// Get length from the array-like object
		var length int
		if obj := arrayLike.AsPlainObject(); obj != nil {
			if lenVal, ok := obj.GetOwn("length"); ok && lenVal.IsNumber() {
				length = int(lenVal.ToFloat())
			}
		}

		if currentIndex >= length {
			// Iterator is exhausted
			result.SetOwnNonEnumerable("value", vm.Undefined)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(true))
		} else {
			// Get value at current index
			var val vm.Value = vm.Undefined
			if obj := arrayLike.AsPlainObject(); obj != nil {
				indexStr := fmt.Sprintf("%d", currentIndex)
				if v, ok := obj.GetOwn(indexStr); ok {
					val = v
				}
			}
			result.SetOwnNonEnumerable("value", val)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(false))
			currentIndex++
		}

		return vm.NewValueFromPlainObject(result), nil
	}))

	// Add [Symbol.iterator] that returns the iterator itself
	iterSelfFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(fnArgs []vm.Value) (vm.Value, error) {
		return iteratorVal, nil
	})
	w, e, c := true, false, true
	iterator.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), iterSelfFn, &w, &e, &c)

	return iteratorVal
}

// createArrayKeysIterator creates an iterator that yields array indices
func createArrayKeysIterator(vmInstance *vm.VM, arrayLike vm.Value) vm.Value {
	iterator := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	iteratorVal := vm.NewValueFromPlainObject(iterator)
	currentIndex := 0

	iterator.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

		// Get length from the array or array-like object
		var length int
		if arrayLike.Type() == vm.TypeArray {
			length = arrayLike.AsArray().Length()
		} else if obj := arrayLike.AsPlainObject(); obj != nil {
			if lenVal, ok := obj.GetOwn("length"); ok && lenVal.IsNumber() {
				length = int(lenVal.ToFloat())
			}
		}

		if currentIndex >= length {
			result.SetOwnNonEnumerable("value", vm.Undefined)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(true))
		} else {
			result.SetOwnNonEnumerable("value", vm.Number(float64(currentIndex)))
			result.SetOwnNonEnumerable("done", vm.BooleanValue(false))
			currentIndex++
		}

		return vm.NewValueFromPlainObject(result), nil
	}))

	// Add [Symbol.iterator] that returns the iterator itself
	iterSelfFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(fnArgs []vm.Value) (vm.Value, error) {
		return iteratorVal, nil
	})
	w, e, c := true, false, true
	iterator.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), iterSelfFn, &w, &e, &c)

	return iteratorVal
}

// createArrayEntriesIterator creates an iterator that yields [index, value] pairs
func createArrayEntriesIterator(vmInstance *vm.VM, arrayLike vm.Value) vm.Value {
	iterator := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	iteratorVal := vm.NewValueFromPlainObject(iterator)
	currentIndex := 0

	iterator.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

		// Get length and value from the array or array-like object
		var length int
		var getValue func(int) vm.Value

		if arrayLike.Type() == vm.TypeArray {
			arr := arrayLike.AsArray()
			length = arr.Length()
			getValue = func(i int) vm.Value { return arr.Get(i) }
		} else if obj := arrayLike.AsPlainObject(); obj != nil {
			if lenVal, ok := obj.GetOwn("length"); ok && lenVal.IsNumber() {
				length = int(lenVal.ToFloat())
			}
			getValue = func(i int) vm.Value {
				if v, ok := obj.GetOwn(fmt.Sprintf("%d", i)); ok {
					return v
				}
				return vm.Undefined
			}
		} else {
			length = 0
			getValue = func(i int) vm.Value { return vm.Undefined }
		}

		if currentIndex >= length {
			result.SetOwnNonEnumerable("value", vm.Undefined)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(true))
		} else {
			// Create [index, value] pair array
			pair := vm.NewArray()
			pairArr := pair.AsArray()
			pairArr.Append(vm.Number(float64(currentIndex)))
			pairArr.Append(getValue(currentIndex))
			result.SetOwnNonEnumerable("value", pair)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(false))
			currentIndex++
		}

		return vm.NewValueFromPlainObject(result), nil
	}))

	// Add [Symbol.iterator] that returns the iterator itself
	iterSelfFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(fnArgs []vm.Value) (vm.Value, error) {
		return iteratorVal, nil
	})
	w, e, c := true, false, true
	iterator.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), iterSelfFn, &w, &e, &c)

	return iteratorVal
}

// createAsyncArrayIterator creates an async iterator object for array iteration
// This wraps array iteration to return promises (for await...of support)
func createAsyncArrayIterator(vmInstance *vm.VM, array *vm.ArrayObject) vm.Value {
	// Create iterator object inheriting from Object.prototype
	iterator := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Iterator state: current index
	currentIndex := 0

	// Add next() method to iterator - returns Promise<{value, done}>
	iterator.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		// Create iterator result object {value, done}
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

		if currentIndex >= array.Length() {
			// Iterator is exhausted
			result.SetOwnNonEnumerable("value", vm.Undefined)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(true))
		} else {
			// Return current element and advance
			val := array.Get(currentIndex)
			result.SetOwnNonEnumerable("value", val)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(false))
			currentIndex++
		}

		// Wrap result in a resolved promise (async iterator protocol)
		resultVal := vm.NewValueFromPlainObject(result)
		return vmInstance.NewResolvedPromise(resultVal), nil
	}))

	return vm.NewValueFromPlainObject(iterator)
}

// Helper methods for creating generic array method types

// createGenericMethod creates a generic method with a single type parameter T
func (a *ArrayInitializer) createGenericMethod(name string, tParam *types.TypeParameter, methodType types.Type) types.Type {
	return &types.GenericType{
		Name:           name,
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           methodType,
	}
}

// createGenericMapMethod creates the special map method that has two type parameters T and U
func (a *ArrayInitializer) createGenericMapMethod(tParam *types.TypeParameter) types.Type {
	// For map, we need both T (input element type) and U (output element type)
	uParam := &types.TypeParameter{Name: "U", Constraint: nil, Index: 1}
	uType := &types.TypeParameterType{Parameter: uParam}
	tType := &types.TypeParameterType{Parameter: tParam}
	tArrayType := &types.ArrayType{ElementType: tType}
	uArrayType := &types.ArrayType{ElementType: uType}

	// map<U>((value: T, index?: number, array?: T[]) => U): U[]
	callbackType := types.NewOptionalFunction(
		[]types.Type{tType, types.Number, tArrayType},
		uType,
		[]bool{false, true, true})

	methodType := types.NewSimpleFunction([]types.Type{callbackType}, uArrayType)

	return &types.GenericType{
		Name:           "map",
		TypeParameters: []*types.TypeParameter{tParam, uParam},
		Body:           methodType,
	}
}
