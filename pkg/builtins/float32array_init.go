package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

type Float32ArrayInitializer struct{}

func (f *Float32ArrayInitializer) Name() string {
	return "Float32Array"
}

func (f *Float32ArrayInitializer) Priority() int {
	return 422 // After Int32Array
}

func (f *Float32ArrayInitializer) InitTypes(ctx *TypeContext) error {
	// Create Float32Array.prototype type
	float32ArrayProtoType := types.NewObjectType().
		WithProperty("buffer", types.Any). // Reference to underlying ArrayBuffer
		WithProperty("byteLength", types.Number).
		WithProperty("byteOffset", types.Number).
		WithProperty("length", types.Number).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("set", types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Undefined)).
		WithProperty("subarray", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true})).
		WithProperty("slice", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true}))

	// Create Float32Array constructor type with multiple overloads
	float32ArrayCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Number}, float32ArrayProtoType).                                // Float32Array(length)
		WithSimpleCallSignature([]types.Type{types.Any}, float32ArrayProtoType).                                   // Float32Array(buffer, byteOffset?, length?)
		WithSimpleCallSignature([]types.Type{&types.ArrayType{ElementType: types.Number}}, float32ArrayProtoType). // Float32Array(array)
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("from", types.NewSimpleFunction([]types.Type{types.Any}, float32ArrayProtoType)).
		WithProperty("of", types.NewSimpleFunction([]types.Type{}, float32ArrayProtoType)).
		WithProperty("prototype", float32ArrayProtoType)

	return ctx.DefineGlobal("Float32Array", float32ArrayCtorType)
}

func (f *Float32ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create Float32Array.prototype inheriting from Object.prototype
	float32ArrayProto := vm.NewObject(objectProto).AsPlainObject()

	// Add prototype properties
	float32ArrayProto.SetOwnNonEnumerable("BYTES_PER_ELEMENT", vm.Number(4))

	// Add buffer getter
	float32ArrayProto.SetOwnNonEnumerable("buffer", vm.NewNativeFunction(0, false, "get buffer", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if ta := thisArray.AsTypedArray(); ta != nil {
			return vm.Value{}, nil // TODO: Return proper ArrayBuffer value
		}
		return vm.Undefined, nil
	}))

	// Add byteLength getter
	float32ArrayProto.SetOwnNonEnumerable("byteLength", vm.NewNativeFunction(0, false, "get byteLength", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if ta := thisArray.AsTypedArray(); ta != nil {
			return vm.Number(float64(ta.GetByteLength())), nil
		}
		return vm.Undefined, nil
	}))

	// Add byteOffset getter
	float32ArrayProto.SetOwnNonEnumerable("byteOffset", vm.NewNativeFunction(0, false, "get byteOffset", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if ta := thisArray.AsTypedArray(); ta != nil {
			return vm.Number(float64(ta.GetByteOffset())), nil
		}
		return vm.Undefined, nil
	}))

	// Add length getter
	float32ArrayProto.SetOwnNonEnumerable("length", vm.NewNativeFunction(0, false, "get length", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if ta := thisArray.AsTypedArray(); ta != nil {
			return vm.Number(float64(ta.GetLength())), nil
		}
		return vm.Undefined, nil
	}))

	// Add set method
	float32ArrayProto.SetOwnNonEnumerable("set", vm.NewNativeFunction(2, false, "set", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta := thisArray.AsTypedArray()
		if ta == nil || len(args) == 0 {
			return vm.Undefined, nil
		}

		source := args[0]
		offset := 0
		if len(args) > 1 {
			offset = int(args[1].ToFloat())
		}

		// Handle array-like source
		if source.Type() == vm.TypeArray {
			sourceArray := source.AsArray()
			for i := 0; i < sourceArray.Length() && offset+i < ta.GetLength(); i++ {
				ta.SetElement(offset+i, sourceArray.Get(i))
			}
		} else if sourceTypedArray := source.AsTypedArray(); sourceTypedArray != nil {
			for i := 0; i < sourceTypedArray.GetLength() && offset+i < ta.GetLength(); i++ {
				ta.SetElement(offset+i, sourceTypedArray.GetElement(i))
			}
		}

		return vm.Undefined, nil
	}))

	// Add fill method
	float32ArrayProto.SetOwnNonEnumerable("fill", vm.NewNativeFunction(3, false, "fill", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.Undefined, nil
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

	// Add subarray method
	float32ArrayProto.SetOwnNonEnumerable("subarray", vm.NewNativeFunction(2, false, "subarray", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.Undefined, nil
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
		byteStart := ta.GetByteOffset() + start*4 // 4 bytes per Float32 element
		length := end - start
		return vm.NewTypedArray(vm.TypedArrayFloat32, ta.GetBuffer(), byteStart, length), nil
	}))

	// Add slice method (creates new array)
	float32ArrayProto.SetOwnNonEnumerable("slice", vm.NewNativeFunction(2, false, "slice", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		ta := thisArray.AsTypedArray()
		if ta == nil {
			return vm.Undefined, nil
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
		newArray := vm.NewTypedArray(vm.TypedArrayFloat32, length, 0, 0)
		if newTA := newArray.AsTypedArray(); newTA != nil {
			for i := 0; i < length; i++ {
				newTA.SetElement(i, ta.GetElement(start+i))
			}
		}

		return newArray, nil
	}))

	// Create Float32Array constructor
	ctorWithProps := vm.NewNativeFunctionWithProps(-1, true, "Float32Array", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayFloat32, 0, 0, 0), nil
		}

		arg := args[0]

		// Handle different constructor patterns
		if arg.IsNumber() {
			// Float32Array(length)
			length := int(arg.ToFloat())
			if length < 0 {
				// Should throw RangeError
				return vm.Undefined, nil
			}
			return vm.NewTypedArray(vm.TypedArrayFloat32, length, 0, 0), nil
		}

		if buffer := arg.AsArrayBuffer(); buffer != nil {
			// Float32Array(buffer, byteOffset?, length?)
			byteOffset := 0
			if len(args) > 1 {
				byteOffset = int(args[1].ToFloat())
			}

			length := -1 // Use remaining buffer
			if len(args) > 2 {
				length = int(args[2].ToFloat())
			}

			return vm.NewTypedArray(vm.TypedArrayFloat32, buffer, byteOffset, length), nil
		}

		if sourceArray := arg.AsArray(); sourceArray != nil {
			// Float32Array(array)
			values := make([]vm.Value, sourceArray.Length())
			for i := 0; i < sourceArray.Length(); i++ {
				values[i] = sourceArray.Get(i)
			}
			return vm.NewTypedArray(vm.TypedArrayFloat32, values, 0, 0), nil
		}

		// Default case
		return vm.NewTypedArray(vm.TypedArrayFloat32, 0, 0, 0), nil
	})

	// Add prototype property
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(float32ArrayProto))

	// Add static properties and methods
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("BYTES_PER_ELEMENT", vm.Number(4))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayFloat32, 0, 0, 0), nil
		}

		source := args[0]
		if sourceArray := source.AsArray(); sourceArray != nil {
			values := make([]vm.Value, sourceArray.Length())
			for i := 0; i < sourceArray.Length(); i++ {
				values[i] = sourceArray.Get(i)
			}
			return vm.NewTypedArray(vm.TypedArrayFloat32, values, 0, 0), nil
		}

		return vm.NewTypedArray(vm.TypedArrayFloat32, 0, 0, 0), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		return vm.NewTypedArray(vm.TypedArrayFloat32, args, 0, 0), nil
	}))

	// Set constructor property on prototype
	float32ArrayProto.SetOwnNonEnumerable("constructor", ctorWithProps)

	// Add Symbol.iterator implementation for typed arrays
	iterFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if thisArray.AsTypedArray() == nil {
			return vm.Undefined, nil
		}
		// Create a typed array iterator object (reuse array iterator logic)
		return createTypedArrayIterator(vmInstance, thisArray), nil
	})
	// Register [Symbol.iterator] using native symbol key
	float32ArrayProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), iterFn, nil, nil, nil)

	// Set Float32Array prototype in VM
	vmInstance.Float32ArrayPrototype = vm.NewValueFromPlainObject(float32ArrayProto)

	// Register Float32Array constructor as global
	return ctx.DefineGlobal("Float32Array", ctorWithProps)
}

// createTypedArrayIterator creates an iterator for typed arrays
func createTypedArrayIterator(vmInstance *vm.VM, typedArray vm.Value) vm.Value {
	index := 0
	ta := typedArray.AsTypedArray()
	length := ta.GetLength()

	// Create iterator object with next() method
	iteratorObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	iteratorObj.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		// Create result object {value, done}
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

		if index >= length {
			result.SetOwnNonEnumerable("done", vm.BooleanValue(true))
			result.SetOwnNonEnumerable("value", vm.Undefined)
		} else {
			val := ta.GetElement(index)
			index++
			result.SetOwnNonEnumerable("done", vm.BooleanValue(false))
			result.SetOwnNonEnumerable("value", val)
		}

		return vm.NewValueFromPlainObject(result), nil
	}))

	return vm.NewValueFromPlainObject(iteratorObj)
}
