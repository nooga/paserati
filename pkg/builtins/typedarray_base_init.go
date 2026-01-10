package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// TypedArrayInitializer sets up the abstract %TypedArray% intrinsic.
// This is the parent of all TypedArray constructors (Int8Array, Uint8Array, etc.)
type TypedArrayInitializer struct{}

func (i *TypedArrayInitializer) Name() string {
	return "TypedArray"
}

func (i *TypedArrayInitializer) Priority() int {
	return 415 // Before individual TypedArrays but after ArrayBuffer
}

func (i *TypedArrayInitializer) InitTypes(ctx *TypeContext) error {
	// Create TypedArray.prototype type with all common methods
	typedArrayProtoType := types.NewObjectType().
		WithProperty("buffer", types.Any).
		WithProperty("byteLength", types.Number).
		WithProperty("byteOffset", types.Number).
		WithProperty("length", types.Number).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("at", types.NewOptionalFunction([]types.Type{types.Number}, types.Any, []bool{false})).
		WithProperty("copyWithin", types.NewOptionalFunction([]types.Type{types.Number, types.Number, types.Number}, types.Any, []bool{false, false, true})).
		WithProperty("entries", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("every", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("fill", types.NewOptionalFunction([]types.Type{types.Any, types.Number, types.Number}, types.Any, []bool{false, true, true})).
		WithProperty("filter", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("find", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("findIndex", types.NewSimpleFunction([]types.Type{types.Any}, types.Number)).
		WithProperty("forEach", types.NewSimpleFunction([]types.Type{types.Any}, types.Undefined)).
		WithProperty("includes", types.NewOptionalFunction([]types.Type{types.Any, types.Number}, types.Boolean, []bool{false, true})).
		WithProperty("indexOf", types.NewOptionalFunction([]types.Type{types.Any, types.Number}, types.Number, []bool{false, true})).
		WithProperty("join", types.NewOptionalFunction([]types.Type{types.String}, types.String, []bool{true})).
		WithProperty("keys", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("lastIndexOf", types.NewOptionalFunction([]types.Type{types.Any, types.Number}, types.Number, []bool{false, true})).
		WithProperty("map", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("reduce", types.NewOptionalFunction([]types.Type{types.Any, types.Any}, types.Any, []bool{false, true})).
		WithProperty("reduceRight", types.NewOptionalFunction([]types.Type{types.Any, types.Any}, types.Any, []bool{false, true})).
		WithProperty("reverse", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("set", types.NewOptionalFunction([]types.Type{types.Any, types.Number}, types.Undefined, []bool{false, true})).
		WithProperty("slice", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true})).
		WithProperty("some", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("sort", types.NewOptionalFunction([]types.Type{types.Any}, types.Any, []bool{true})).
		WithProperty("subarray", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true})).
		WithProperty("toLocaleString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("values", types.NewSimpleFunction([]types.Type{}, types.Any))

	// Create TypedArray constructor type (abstract - cannot be called directly)
	typedArrayCtorType := types.NewObjectType().
		WithProperty("from", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("of", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("prototype", typedArrayProtoType)

	return ctx.DefineGlobal("TypedArray", typedArrayCtorType)
}

func (i *TypedArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create TypedArray.prototype inheriting from Object.prototype
	typedArrayProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Set up all the common TypedArray prototype methods with proper error checking
	setupTypedArrayPrototypeWithErrors(typedArrayProto, vmInstance)

	// Store the prototype in VM for inheritance
	vmInstance.TypedArrayPrototype = vm.NewValueFromPlainObject(typedArrayProto)

	// Create the abstract TypedArray constructor
	// Calling TypedArray() directly should throw a TypeError
	typedArrayCtor := vm.NewConstructorWithProps(-1, true, "TypedArray", func(args []vm.Value) (vm.Value, error) {
		return vm.Undefined, vmInstance.NewTypeError("Abstract class TypedArray not directly constructable")
	})

	// Add prototype property
	typedArrayCtor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(typedArrayProto))

	// Add TypedArray.from() static method
	typedArrayCtor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		// TypedArray.from() when called on the abstract TypedArray should throw
		return vm.Undefined, vmInstance.NewTypeError("TypedArray.from requires a valid TypedArray constructor")
	}))

	// Add TypedArray.of() static method
	typedArrayCtor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		// TypedArray.of() when called on the abstract TypedArray should throw
		return vm.Undefined, vmInstance.NewTypeError("TypedArray.of requires a valid TypedArray constructor")
	}))

	// Set constructor property on prototype
	typedArrayProto.SetOwnNonEnumerable("constructor", typedArrayCtor)

	// Register TypedArray constructor as global
	return ctx.DefineGlobal("TypedArray", typedArrayCtor)
}

// setupTypedArrayPrototypeWithErrors adds common TypedArray prototype methods with proper error checking.
func setupTypedArrayPrototypeWithErrors(proto *vm.PlainObject, vmInstance *vm.VM) {
	// Helper function to validate TypedArray 'this' value
	validateTypedArray := func(thisVal vm.Value, methodName string) (*vm.TypedArrayObject, error) {
		ta := thisVal.AsTypedArray()
		if ta == nil {
			return nil, vmInstance.NewTypeError(methodName + " called on non-TypedArray object")
		}
		if ta.GetBuffer().IsDetached() {
			return nil, vmInstance.NewTypeError("Cannot perform " + methodName + " on a detached ArrayBuffer")
		}
		return ta, nil
	}

	// at(index) - returns element at index (supports negative indices)
	proto.SetOwnNonEnumerable("at", vm.NewNativeFunction(1, false, "at", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.at")
		if err != nil {
			return vm.Undefined, err
		}

		length := ta.GetLength()
		if length == 0 || len(args) == 0 {
			return vm.Undefined, nil
		}

		index := int(args[0].ToFloat())
		if index < 0 {
			index = length + index
		}
		if index < 0 || index >= length {
			return vm.Undefined, nil
		}

		return ta.GetElement(index), nil
	}))

	// indexOf(searchElement, fromIndex?) - returns first index of element
	proto.SetOwnNonEnumerable("indexOf", vm.NewNativeFunction(2, false, "indexOf", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.indexOf")
		if err != nil {
			return vm.Undefined, err
		}

		if len(args) == 0 {
			return vm.Number(-1), nil
		}

		searchElement := args[0]
		fromIndex := 0
		if len(args) > 1 {
			fromIndex = int(args[1].ToFloat())
		}

		length := ta.GetLength()
		if fromIndex < 0 {
			fromIndex = length + fromIndex
			if fromIndex < 0 {
				fromIndex = 0
			}
		}

		for i := fromIndex; i < length; i++ {
			elem := ta.GetElement(i)
			if elem.StrictlyEquals(searchElement) {
				return vm.Number(float64(i)), nil
			}
		}

		return vm.Number(-1), nil
	}))

	// lastIndexOf(searchElement, fromIndex?) - returns last index of element
	proto.SetOwnNonEnumerable("lastIndexOf", vm.NewNativeFunction(2, false, "lastIndexOf", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.lastIndexOf")
		if err != nil {
			return vm.Undefined, err
		}

		if len(args) == 0 {
			return vm.Number(-1), nil
		}

		searchElement := args[0]
		length := ta.GetLength()
		fromIndex := length - 1
		if len(args) > 1 {
			fromIndex = int(args[1].ToFloat())
		}
		if fromIndex < 0 {
			fromIndex = length + fromIndex
		}
		if fromIndex >= length {
			fromIndex = length - 1
		}

		for i := fromIndex; i >= 0; i-- {
			elem := ta.GetElement(i)
			if elem.StrictlyEquals(searchElement) {
				return vm.Number(float64(i)), nil
			}
		}

		return vm.Number(-1), nil
	}))

	// includes(searchElement, fromIndex?) - returns true if element is found
	proto.SetOwnNonEnumerable("includes", vm.NewNativeFunction(2, false, "includes", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.includes")
		if err != nil {
			return vm.Undefined, err
		}

		if len(args) == 0 {
			return vm.False, nil
		}

		searchElement := args[0]
		fromIndex := 0
		if len(args) > 1 {
			fromIndex = int(args[1].ToFloat())
		}

		length := ta.GetLength()
		if fromIndex < 0 {
			fromIndex = length + fromIndex
			if fromIndex < 0 {
				fromIndex = 0
			}
		}

		for i := fromIndex; i < length; i++ {
			elem := ta.GetElement(i)
			if elem.StrictlyEquals(searchElement) {
				return vm.True, nil
			}
		}

		return vm.False, nil
	}))

	// join(separator?) - joins elements into a string
	proto.SetOwnNonEnumerable("join", vm.NewNativeFunction(1, false, "join", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.join")
		if err != nil {
			return vm.Undefined, err
		}

		separator := ","
		if len(args) > 0 && !args[0].IsUndefined() {
			separator = args[0].ToString()
		}

		length := ta.GetLength()
		if length == 0 {
			return vm.NewString(""), nil
		}

		result := ""
		for i := 0; i < length; i++ {
			if i > 0 {
				result += separator
			}
			elem := ta.GetElement(i)
			if !elem.IsUndefined() && elem.Type() != vm.TypeNull {
				result += elem.ToString()
			}
		}

		return vm.NewString(result), nil
	}))

	// toString() - joins elements with comma
	proto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.toString")
		if err != nil {
			return vm.Undefined, err
		}

		length := ta.GetLength()
		if length == 0 {
			return vm.NewString(""), nil
		}

		result := ""
		for i := 0; i < length; i++ {
			if i > 0 {
				result += ","
			}
			elem := ta.GetElement(i)
			if !elem.IsUndefined() && elem.Type() != vm.TypeNull {
				result += elem.ToString()
			}
		}

		return vm.NewString(result), nil
	}))

	// reverse() - reverses in place
	proto.SetOwnNonEnumerable("reverse", vm.NewNativeFunction(0, false, "reverse", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.reverse")
		if err != nil {
			return vm.Undefined, err
		}

		length := ta.GetLength()
		for i := 0; i < length/2; i++ {
			j := length - 1 - i
			temp := ta.GetElement(i)
			ta.SetElement(i, ta.GetElement(j))
			ta.SetElement(j, temp)
		}

		return thisArray, nil
	}))

	// forEach(callback, thisArg?) - calls callback for each element
	proto.SetOwnNonEnumerable("forEach", vm.NewNativeFunction(2, false, "forEach", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.forEach")
		if err != nil {
			return vm.Undefined, err
		}

		if len(args) == 0 || !args[0].IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("%TypedArray%.prototype.forEach callback is not a function")
		}

		callback := args[0]
		length := ta.GetLength()
		for i := 0; i < length; i++ {
			elem := ta.GetElement(i)
			_, err := vmInstance.Call(callback, vm.Undefined, []vm.Value{elem, vm.Number(float64(i)), thisArray})
			if err != nil {
				return vm.Undefined, err
			}
		}

		return vm.Undefined, nil
	}))

	// every(callback, thisArg?) - returns true if callback returns true for all elements
	proto.SetOwnNonEnumerable("every", vm.NewNativeFunction(2, false, "every", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.every")
		if err != nil {
			return vm.Undefined, err
		}

		if len(args) == 0 || !args[0].IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("%TypedArray%.prototype.every callback is not a function")
		}

		callback := args[0]
		length := ta.GetLength()
		for i := 0; i < length; i++ {
			elem := ta.GetElement(i)
			result, err := vmInstance.Call(callback, vm.Undefined, []vm.Value{elem, vm.Number(float64(i)), thisArray})
			if err != nil {
				return vm.Undefined, err
			}
			if result.IsFalsey() {
				return vm.False, nil
			}
		}

		return vm.True, nil
	}))

	// some(callback, thisArg?) - returns true if callback returns true for any element
	proto.SetOwnNonEnumerable("some", vm.NewNativeFunction(2, false, "some", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.some")
		if err != nil {
			return vm.Undefined, err
		}

		if len(args) == 0 || !args[0].IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("%TypedArray%.prototype.some callback is not a function")
		}

		callback := args[0]
		length := ta.GetLength()
		for i := 0; i < length; i++ {
			elem := ta.GetElement(i)
			result, err := vmInstance.Call(callback, vm.Undefined, []vm.Value{elem, vm.Number(float64(i)), thisArray})
			if err != nil {
				return vm.Undefined, err
			}
			if !result.IsFalsey() {
				return vm.True, nil
			}
		}

		return vm.False, nil
	}))

	// find(callback, thisArg?) - returns first element where callback returns true
	proto.SetOwnNonEnumerable("find", vm.NewNativeFunction(2, false, "find", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.find")
		if err != nil {
			return vm.Undefined, err
		}

		if len(args) == 0 || !args[0].IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("%TypedArray%.prototype.find callback is not a function")
		}

		callback := args[0]
		length := ta.GetLength()
		for i := 0; i < length; i++ {
			elem := ta.GetElement(i)
			result, err := vmInstance.Call(callback, vm.Undefined, []vm.Value{elem, vm.Number(float64(i)), thisArray})
			if err != nil {
				return vm.Undefined, err
			}
			if !result.IsFalsey() {
				return elem, nil
			}
		}

		return vm.Undefined, nil
	}))

	// findIndex(callback, thisArg?) - returns first index where callback returns true
	proto.SetOwnNonEnumerable("findIndex", vm.NewNativeFunction(2, false, "findIndex", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.findIndex")
		if err != nil {
			return vm.Undefined, err
		}

		if len(args) == 0 || !args[0].IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("%TypedArray%.prototype.findIndex callback is not a function")
		}

		callback := args[0]
		length := ta.GetLength()
		for i := 0; i < length; i++ {
			elem := ta.GetElement(i)
			result, err := vmInstance.Call(callback, vm.Undefined, []vm.Value{elem, vm.Number(float64(i)), thisArray})
			if err != nil {
				return vm.Undefined, err
			}
			if !result.IsFalsey() {
				return vm.Number(float64(i)), nil
			}
		}

		return vm.Number(-1), nil
	}))

	// filter(callback, thisArg?) - returns new array with elements where callback returns true
	proto.SetOwnNonEnumerable("filter", vm.NewNativeFunction(2, false, "filter", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.filter")
		if err != nil {
			return vm.Undefined, err
		}

		if len(args) == 0 || !args[0].IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("%TypedArray%.prototype.filter callback is not a function")
		}

		callback := args[0]
		length := ta.GetLength()
		kept := make([]vm.Value, 0)

		for i := 0; i < length; i++ {
			elem := ta.GetElement(i)
			result, err := vmInstance.Call(callback, vm.Undefined, []vm.Value{elem, vm.Number(float64(i)), thisArray})
			if err != nil {
				return vm.Undefined, err
			}
			if !result.IsFalsey() {
				kept = append(kept, elem)
			}
		}

		// Create new TypedArray of same type with filtered elements
		return vm.NewTypedArray(ta.GetElementType(), kept, 0, 0), nil
	}))

	// map(callback, thisArg?) - returns new array with mapped elements
	proto.SetOwnNonEnumerable("map", vm.NewNativeFunction(2, false, "map", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.map")
		if err != nil {
			return vm.Undefined, err
		}

		if len(args) == 0 || !args[0].IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("%TypedArray%.prototype.map callback is not a function")
		}

		callback := args[0]
		length := ta.GetLength()
		mapped := make([]vm.Value, length)

		for i := 0; i < length; i++ {
			elem := ta.GetElement(i)
			result, err := vmInstance.Call(callback, vm.Undefined, []vm.Value{elem, vm.Number(float64(i)), thisArray})
			if err != nil {
				return vm.Undefined, err
			}
			mapped[i] = result
		}

		// Create new TypedArray of same type with mapped elements
		return vm.NewTypedArray(ta.GetElementType(), mapped, 0, 0), nil
	}))

	// reduce(callback, initialValue?) - reduces array to single value
	proto.SetOwnNonEnumerable("reduce", vm.NewNativeFunction(2, false, "reduce", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.reduce")
		if err != nil {
			return vm.Undefined, err
		}

		if len(args) == 0 || !args[0].IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("%TypedArray%.prototype.reduce callback is not a function")
		}

		callback := args[0]
		length := ta.GetLength()
		var accumulator vm.Value
		startIndex := 0

		if len(args) > 1 {
			accumulator = args[1]
		} else {
			if length == 0 {
				return vm.Undefined, vmInstance.NewTypeError("Reduce of empty TypedArray with no initial value")
			}
			accumulator = ta.GetElement(0)
			startIndex = 1
		}

		for i := startIndex; i < length; i++ {
			elem := ta.GetElement(i)
			result, err := vmInstance.Call(callback, vm.Undefined, []vm.Value{accumulator, elem, vm.Number(float64(i)), thisArray})
			if err != nil {
				return vm.Undefined, err
			}
			accumulator = result
		}

		return accumulator, nil
	}))

	// reduceRight(callback, initialValue?) - reduces array from right to single value
	proto.SetOwnNonEnumerable("reduceRight", vm.NewNativeFunction(2, false, "reduceRight", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.reduceRight")
		if err != nil {
			return vm.Undefined, err
		}

		if len(args) == 0 || !args[0].IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("%TypedArray%.prototype.reduceRight callback is not a function")
		}

		callback := args[0]
		length := ta.GetLength()
		var accumulator vm.Value
		startIndex := length - 1

		if len(args) > 1 {
			accumulator = args[1]
		} else {
			if length == 0 {
				return vm.Undefined, vmInstance.NewTypeError("Reduce of empty TypedArray with no initial value")
			}
			accumulator = ta.GetElement(length - 1)
			startIndex = length - 2
		}

		for i := startIndex; i >= 0; i-- {
			elem := ta.GetElement(i)
			result, err := vmInstance.Call(callback, vm.Undefined, []vm.Value{accumulator, elem, vm.Number(float64(i)), thisArray})
			if err != nil {
				return vm.Undefined, err
			}
			accumulator = result
		}

		return accumulator, nil
	}))

	// copyWithin(target, start, end?) - copies part of array to another location
	proto.SetOwnNonEnumerable("copyWithin", vm.NewNativeFunction(3, false, "copyWithin", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.copyWithin")
		if err != nil {
			return vm.Undefined, err
		}

		length := ta.GetLength()
		if len(args) == 0 {
			return thisArray, nil
		}

		target := int(args[0].ToFloat())
		if target < 0 {
			target = length + target
			if target < 0 {
				target = 0
			}
		}
		if target >= length {
			return thisArray, nil
		}

		start := 0
		if len(args) > 1 {
			start = int(args[1].ToFloat())
		}
		if start < 0 {
			start = length + start
			if start < 0 {
				start = 0
			}
		}

		end := length
		if len(args) > 2 && !args[2].IsUndefined() {
			end = int(args[2].ToFloat())
		}
		if end < 0 {
			end = length + end
		}
		if end > length {
			end = length
		}

		count := end - start
		if target+count > length {
			count = length - target
		}

		// Copy to temporary to handle overlapping
		temp := make([]vm.Value, count)
		for i := 0; i < count; i++ {
			temp[i] = ta.GetElement(start + i)
		}

		for i := 0; i < count; i++ {
			ta.SetElement(target+i, temp[i])
		}

		return thisArray, nil
	}))

	// fill(value, start?, end?) - fills array with value
	proto.SetOwnNonEnumerable("fill", vm.NewNativeFunction(3, false, "fill", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.fill")
		if err != nil {
			return vm.Undefined, err
		}

		value := vm.Undefined
		if len(args) > 0 {
			value = args[0]
		}
		start := 0
		end := ta.GetLength()
		if len(args) > 1 && !args[1].IsUndefined() {
			start = int(args[1].ToFloat())
			if start < 0 {
				start = ta.GetLength() + start
			}
			if start < 0 {
				start = 0
			}
		}
		if len(args) > 2 && !args[2].IsUndefined() {
			end = int(args[2].ToFloat())
			if end < 0 {
				end = ta.GetLength() + end
			}
			if end < 0 {
				end = 0
			}
			if end > ta.GetLength() {
				end = ta.GetLength()
			}
		}
		for i := start; i < end; i++ {
			ta.SetElement(i, value)
		}
		return thisArray, nil
	}))

	// entries() - returns iterator of [index, value] pairs
	proto.SetOwnNonEnumerable("entries", vm.NewNativeFunction(0, false, "entries", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.entries")
		if err != nil {
			return vm.Undefined, err
		}

		index := 0
		length := ta.GetLength()

		iteratorObj := vm.NewObject(vm.Undefined).AsPlainObject()
		iteratorObj.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
			if index >= length {
				result := vm.NewObject(vm.Undefined).AsPlainObject()
				result.SetOwn("value", vm.Undefined)
				result.SetOwn("done", vm.True)
				return vm.NewValueFromPlainObject(result), nil
			}

			elem := ta.GetElement(index)
			pair := vm.NewArrayWithArgs([]vm.Value{vm.Number(float64(index)), elem})
			index++

			result := vm.NewObject(vm.Undefined).AsPlainObject()
			result.SetOwn("value", pair)
			result.SetOwn("done", vm.False)
			return vm.NewValueFromPlainObject(result), nil
		}))

		return vm.NewValueFromPlainObject(iteratorObj), nil
	}))

	// keys() - returns iterator of indices
	proto.SetOwnNonEnumerable("keys", vm.NewNativeFunction(0, false, "keys", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.keys")
		if err != nil {
			return vm.Undefined, err
		}

		index := 0
		length := ta.GetLength()

		iteratorObj := vm.NewObject(vm.Undefined).AsPlainObject()
		iteratorObj.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
			if index >= length {
				result := vm.NewObject(vm.Undefined).AsPlainObject()
				result.SetOwn("value", vm.Undefined)
				result.SetOwn("done", vm.True)
				return vm.NewValueFromPlainObject(result), nil
			}

			value := vm.Number(float64(index))
			index++

			result := vm.NewObject(vm.Undefined).AsPlainObject()
			result.SetOwn("value", value)
			result.SetOwn("done", vm.False)
			return vm.NewValueFromPlainObject(result), nil
		}))

		return vm.NewValueFromPlainObject(iteratorObj), nil
	}))

	// values() - returns iterator of values
	proto.SetOwnNonEnumerable("values", vm.NewNativeFunction(0, false, "values", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.values")
		if err != nil {
			return vm.Undefined, err
		}

		index := 0
		length := ta.GetLength()

		iteratorObj := vm.NewObject(vm.Undefined).AsPlainObject()
		iteratorObj.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
			if index >= length {
				result := vm.NewObject(vm.Undefined).AsPlainObject()
				result.SetOwn("value", vm.Undefined)
				result.SetOwn("done", vm.True)
				return vm.NewValueFromPlainObject(result), nil
			}

			value := ta.GetElement(index)
			index++

			result := vm.NewObject(vm.Undefined).AsPlainObject()
			result.SetOwn("value", value)
			result.SetOwn("done", vm.False)
			return vm.NewValueFromPlainObject(result), nil
		}))

		return vm.NewValueFromPlainObject(iteratorObj), nil
	}))

	// toLocaleString() - joins elements using toLocaleString
	proto.SetOwnNonEnumerable("toLocaleString", vm.NewNativeFunction(0, false, "toLocaleString", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.toLocaleString")
		if err != nil {
			return vm.Undefined, err
		}

		length := ta.GetLength()
		if length == 0 {
			return vm.NewString(""), nil
		}

		result := ""
		for i := 0; i < length; i++ {
			if i > 0 {
				result += ","
			}
			elem := ta.GetElement(i)
			if !elem.IsUndefined() && elem.Type() != vm.TypeNull {
				result += elem.ToString()
			}
		}

		return vm.NewString(result), nil
	}))

	// sort(compareFn?) - sorts array in place
	proto.SetOwnNonEnumerable("sort", vm.NewNativeFunction(1, false, "sort", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.sort")
		if err != nil {
			return vm.Undefined, err
		}

		length := ta.GetLength()
		if length <= 1 {
			return thisArray, nil
		}

		// Copy elements to slice for sorting
		elements := make([]vm.Value, length)
		for i := 0; i < length; i++ {
			elements[i] = ta.GetElement(i)
		}

		var callback vm.Value
		if len(args) > 0 && args[0].IsCallable() {
			callback = args[0]
		}

		// Simple bubble sort (not efficient but correct)
		for i := 0; i < length-1; i++ {
			for j := 0; j < length-i-1; j++ {
				var shouldSwap bool
				if callback.IsCallable() {
					result, err := vmInstance.Call(callback, vm.Undefined, []vm.Value{elements[j], elements[j+1]})
					if err != nil {
						return vm.Undefined, err
					}
					shouldSwap = result.ToFloat() > 0
				} else {
					// Default numeric sort for typed arrays
					shouldSwap = elements[j].ToFloat() > elements[j+1].ToFloat()
				}
				if shouldSwap {
					elements[j], elements[j+1] = elements[j+1], elements[j]
				}
			}
		}

		// Copy back
		for i := 0; i < length; i++ {
			ta.SetElement(i, elements[i])
		}

		return thisArray, nil
	}))

	// set(source, offset?) - copies elements from source
	proto.SetOwnNonEnumerable("set", vm.NewNativeFunction(2, false, "set", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.set")
		if err != nil {
			return vm.Undefined, err
		}

		if len(args) == 0 {
			return vm.Undefined, nil
		}

		source := args[0]
		offset := 0
		if len(args) > 1 {
			offset = int(args[1].ToFloat())
		}

		if offset < 0 {
			return vm.Undefined, vmInstance.NewRangeError("offset is out of bounds")
		}

		// Handle array-like source
		if source.Type() == vm.TypeArray {
			sourceArray := source.AsArray()
			if offset+sourceArray.Length() > ta.GetLength() {
				return vm.Undefined, vmInstance.NewRangeError("source is too large")
			}
			for i := 0; i < sourceArray.Length(); i++ {
				ta.SetElement(offset+i, sourceArray.Get(i))
			}
		} else if sourceTypedArray := source.AsTypedArray(); sourceTypedArray != nil {
			if offset+sourceTypedArray.GetLength() > ta.GetLength() {
				return vm.Undefined, vmInstance.NewRangeError("source is too large")
			}
			for i := 0; i < sourceTypedArray.GetLength(); i++ {
				ta.SetElement(offset+i, sourceTypedArray.GetElement(i))
			}
		}

		return vm.Undefined, nil
	}))

	// subarray(begin?, end?) - returns a new view into the same buffer
	proto.SetOwnNonEnumerable("subarray", vm.NewNativeFunction(2, false, "subarray", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.subarray")
		if err != nil {
			return vm.Undefined, err
		}

		start := 0
		end := ta.GetLength()

		if len(args) > 0 && !args[0].IsUndefined() {
			start = int(args[0].ToFloat())
			if start < 0 {
				start = ta.GetLength() + start
			}
			if start < 0 {
				start = 0
			}
			if start > ta.GetLength() {
				start = ta.GetLength()
			}
		}

		if len(args) > 1 && !args[1].IsUndefined() {
			end = int(args[1].ToFloat())
			if end < 0 {
				end = ta.GetLength() + end
			}
			if end < 0 {
				end = 0
			}
			if end > ta.GetLength() {
				end = ta.GetLength()
			}
		}

		if start > end {
			start = end
		}

		// Create new view into same buffer
		byteStart := ta.GetByteOffset() + start*ta.GetBytesPerElement()
		length := end - start
		return vm.NewTypedArray(ta.GetElementType(), ta.GetBuffer(), byteStart, length), nil
	}))

	// slice(begin?, end?) - returns a new array with copied elements
	proto.SetOwnNonEnumerable("slice", vm.NewNativeFunction(2, false, "slice", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta, err := validateTypedArray(thisArray, "%TypedArray%.prototype.slice")
		if err != nil {
			return vm.Undefined, err
		}

		start := 0
		end := ta.GetLength()

		if len(args) > 0 && !args[0].IsUndefined() {
			start = int(args[0].ToFloat())
			if start < 0 {
				start = ta.GetLength() + start
			}
			if start < 0 {
				start = 0
			}
			if start > ta.GetLength() {
				start = ta.GetLength()
			}
		}

		if len(args) > 1 && !args[1].IsUndefined() {
			end = int(args[1].ToFloat())
			if end < 0 {
				end = ta.GetLength() + end
			}
			if end < 0 {
				end = 0
			}
			if end > ta.GetLength() {
				end = ta.GetLength()
			}
		}

		if start > end {
			start = end
		}

		// Create new array with copied data
		length := end - start
		newArray := vm.NewTypedArray(ta.GetElementType(), length, 0, 0)
		if newTA := newArray.AsTypedArray(); newTA != nil {
			for i := 0; i < length; i++ {
				newTA.SetElement(i, ta.GetElement(start+i))
			}
		}

		return newArray, nil
	}))
}
