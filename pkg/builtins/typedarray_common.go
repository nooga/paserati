package builtins

import (
	"fmt"

	"github.com/nooga/paserati/pkg/vm"
)

// ValidateTypedArrayByteOffset converts byteOffset to integer (calling valueOf if needed)
// and validates that it's aligned to the element size.
// Per ECMAScript spec 22.2.4.5:
// - Step 7: Let offset be ? ToInteger(byteOffset)
// - Step 10: If offset modulo elementSize ≠ 0, throw a RangeError
func ValidateTypedArrayByteOffset(vmInstance *vm.VM, byteOffsetArg vm.Value, elementSize int) (int, error) {
	// If undefined, default to 0
	if byteOffsetArg.IsUndefined() {
		return 0, nil
	}

	// Call ToInteger which properly invokes valueOf() through ToPrimitive
	offset, err := toIntegerOrInfinityWithVM(vmInstance, byteOffsetArg)
	if err != nil {
		return 0, err
	}

	// Step 8: If offset < 0, throw a RangeError
	if offset < 0 {
		return 0, vmInstance.NewRangeError("Start offset is negative")
	}

	// Step 10: If offset modulo elementSize ≠ 0, throw a RangeError
	if elementSize > 1 && offset%elementSize != 0 {
		return 0, vmInstance.NewRangeError(fmt.Sprintf("Start offset of %s should be a multiple of %d", getTypedArrayNameFromElementSize(elementSize), elementSize))
	}

	return offset, nil
}

// getTypedArrayNameFromElementSize returns the TypedArray name based on element size
func getTypedArrayNameFromElementSize(elementSize int) string {
	switch elementSize {
	case 1:
		return "Int8Array" // or Uint8Array/Uint8ClampedArray
	case 2:
		return "Int16Array" // or Uint16Array
	case 4:
		return "Int32Array" // or Uint32Array/Float32Array
	case 8:
		return "Float64Array" // or BigInt64Array/BigUint64Array
	default:
		return "TypedArray"
	}
}

// ValidateTypedArrayBufferAlignment checks that when length is undefined (auto-calculate),
// the remaining buffer bytes after byteOffset is aligned to elementSize.
// Per ECMAScript spec 22.2.4.5 step 13a:
// - If bufferByteLength modulo elementSize ≠ 0, throw a RangeError
func ValidateTypedArrayBufferAlignment(vmInstance *vm.VM, buffer *vm.ArrayBufferObject, byteOffset, elementSize int) error {
	bufferByteLength := len(buffer.GetData())
	remainingBytes := bufferByteLength - byteOffset

	if remainingBytes < 0 {
		return vmInstance.NewRangeError("Start offset is outside the bounds of the buffer")
	}

	if remainingBytes%elementSize != 0 {
		return vmInstance.NewRangeError(fmt.Sprintf("Byte length of %s should be a multiple of %d", getTypedArrayNameFromElementSize(elementSize), elementSize))
	}

	return nil
}

// SetupTypedArrayConstructorProperties sets up the constructor properties with correct descriptors.
// Per ECMAScript spec:
// - BYTES_PER_ELEMENT: { writable: false, enumerable: false, configurable: false }
// - prototype: { writable: false, enumerable: false, configurable: false }
// - length: { writable: false, enumerable: false, configurable: true } - value is 3
func SetupTypedArrayConstructorProperties(ctor vm.Value, proto *vm.PlainObject, bytesPerElement int) {
	props := ctor.AsNativeFunctionWithProps().Properties

	// BYTES_PER_ELEMENT: { writable: false, enumerable: false, configurable: false }
	w, e, c := false, false, false

	props.DefineOwnProperty("BYTES_PER_ELEMENT", vm.Number(float64(bytesPerElement)), &w, &e, &c)

	// prototype: { writable: false, enumerable: false, configurable: false }
	props.DefineOwnProperty("prototype", vm.NewValueFromPlainObject(proto), &w, &e, &c)
}

// SetupTypedArrayPrototypeProperties sets up the prototype properties with correct descriptors.
// Per ECMAScript spec:
// - BYTES_PER_ELEMENT: { writable: false, enumerable: false, configurable: false }
// - buffer, byteLength, byteOffset, length are getters that throw TypeError on non-TypedArray
func SetupTypedArrayPrototypeProperties(proto *vm.PlainObject, vmInstance *vm.VM, bytesPerElement int) {
	// BYTES_PER_ELEMENT: { writable: false, enumerable: false, configurable: false }
	w, e, c := false, false, false
	proto.DefineOwnProperty("BYTES_PER_ELEMENT", vm.Number(float64(bytesPerElement)), &w, &e, &c)

	// buffer getter - throws TypeError if this is not a TypedArray
	bufferGetter := vm.NewNativeFunction(0, false, "get buffer", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.Undefined, vmInstance.NewTypeError("get TypedArray.prototype.buffer called on incompatible receiver")
		}
		// Return the ArrayBuffer as a Value
		return vm.NewArrayBufferFromObject(ta.GetBuffer()), nil
	})
	proto.DefineAccessorProperty("buffer", bufferGetter, true, vm.Undefined, false, &e, &c)

	// byteLength getter - throws TypeError if this is not a TypedArray
	byteLengthGetter := vm.NewNativeFunction(0, false, "get byteLength", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.Undefined, vmInstance.NewTypeError("get TypedArray.prototype.byteLength called on incompatible receiver")
		}
		return vm.Number(float64(ta.GetByteLength())), nil
	})
	proto.DefineAccessorProperty("byteLength", byteLengthGetter, true, vm.Undefined, false, &e, &c)

	// byteOffset getter - throws TypeError if this is not a TypedArray
	byteOffsetGetter := vm.NewNativeFunction(0, false, "get byteOffset", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.Undefined, vmInstance.NewTypeError("get TypedArray.prototype.byteOffset called on incompatible receiver")
		}
		return vm.Number(float64(ta.GetByteOffset())), nil
	})
	proto.DefineAccessorProperty("byteOffset", byteOffsetGetter, true, vm.Undefined, false, &e, &c)

	// length getter - throws TypeError if this is not a TypedArray
	lengthGetter := vm.NewNativeFunction(0, false, "get length", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.Undefined, vmInstance.NewTypeError("get TypedArray.prototype.length called on incompatible receiver")
		}
		return vm.Number(float64(ta.GetLength())), nil
	})
	proto.DefineAccessorProperty("length", lengthGetter, true, vm.Undefined, false, &e, &c)
}

// SetupTypedArrayPrototype adds common TypedArray prototype methods to the given prototype object.
// This should be called after setting up the type-specific properties.
func SetupTypedArrayPrototype(proto *vm.PlainObject, vmInstance *vm.VM) {
	// at(index) - returns element at index (supports negative indices)
	proto.SetOwnNonEnumerable("at", vm.NewNativeFunction(1, false, "at", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.Undefined, nil
		}

		length := ta.GetLength()
		if length == 0 || len(args) == 0 {
			return vm.Undefined, nil
		}

		index, err := toIntegerOrInfinityWithVM(vmInstance, args[0])
		if err != nil {
			if err == ErrVMUnwinding {
				return vm.Undefined, nil
			}
			return vm.Undefined, err
		}
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
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.Number(-1), nil
		}

		if len(args) == 0 {
			return vm.Number(-1), nil
		}

		searchElement := args[0]
		fromIndex := 0
		if len(args) > 1 {
			var err error
			fromIndex, err = toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
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
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.Number(-1), nil
		}

		if len(args) == 0 {
			return vm.Number(-1), nil
		}

		searchElement := args[0]
		length := ta.GetLength()
		fromIndex := length - 1
		if len(args) > 1 {
			var err error
			fromIndex, err = toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
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
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.False, nil
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
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.NewString(""), nil
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
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.NewString(""), nil
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
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.Undefined, nil
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
		ta := thisArray.AsTypedArray()
		if ta == nil || len(args) == 0 {
			return vm.Undefined, nil
		}

		callback := args[0]
		if !callback.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("forEach callback is not a function")
		}

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
		ta := thisArray.AsTypedArray()
		if ta == nil || len(args) == 0 {
			return vm.True, nil
		}

		callback := args[0]
		if !callback.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("every callback is not a function")
		}

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
		ta := thisArray.AsTypedArray()
		if ta == nil || len(args) == 0 {
			return vm.False, nil
		}

		callback := args[0]
		if !callback.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("some callback is not a function")
		}

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
		ta := thisArray.AsTypedArray()
		if ta == nil || len(args) == 0 {
			return vm.Undefined, nil
		}

		callback := args[0]
		if !callback.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("find callback is not a function")
		}

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
		ta := thisArray.AsTypedArray()
		if ta == nil || len(args) == 0 {
			return vm.Number(-1), nil
		}

		callback := args[0]
		if !callback.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("findIndex callback is not a function")
		}

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

	// reduce(callback, initialValue?) - reduces array to single value
	proto.SetOwnNonEnumerable("reduce", vm.NewNativeFunction(2, false, "reduce", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta := thisArray.AsTypedArray()
		if ta == nil || len(args) == 0 {
			return vm.Undefined, nil
		}

		callback := args[0]
		if !callback.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("reduce callback is not a function")
		}

		length := ta.GetLength()
		var accumulator vm.Value
		startIndex := 0

		if len(args) > 1 {
			accumulator = args[1]
		} else {
			if length == 0 {
				return vm.Undefined, vmInstance.NewTypeError("Reduce of empty array with no initial value")
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
		ta := thisArray.AsTypedArray()
		if ta == nil || len(args) == 0 {
			return vm.Undefined, nil
		}

		callback := args[0]
		if !callback.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("reduceRight callback is not a function")
		}

		length := ta.GetLength()
		var accumulator vm.Value
		startIndex := length - 1

		if len(args) > 1 {
			accumulator = args[1]
		} else {
			if length == 0 {
				return vm.Undefined, vmInstance.NewTypeError("Reduce of empty array with no initial value")
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
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.Undefined, nil
		}

		length := ta.GetLength()
		if len(args) == 0 {
			return thisArray, nil
		}

		target, err := toIntegerOrInfinityWithVM(vmInstance, args[0])
		if err != nil {
			if err == ErrVMUnwinding {
				return vm.Undefined, nil
			}
			return vm.Undefined, err
		}
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
		}

		end := length
		if len(args) > 2 && !args[2].IsUndefined() {
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

	// entries() - returns iterator of [index, value] pairs
	proto.SetOwnNonEnumerable("entries", vm.NewNativeFunction(0, false, "entries", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.Undefined, nil
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
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.Undefined, nil
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
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.Undefined, nil
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
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.NewString(""), nil
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
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.Undefined, nil
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
}
