package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

type Float64ArrayInitializer struct{}

func (u *Float64ArrayInitializer) Name() string {
	return "Float64Array"
}

func (u *Float64ArrayInitializer) Priority() int {
	return 423 // After Uint16Array
}

func (u *Float64ArrayInitializer) InitTypes(ctx *TypeContext) error {
	// Create Float64Array.prototype type
	float64ArrayProtoType := types.NewObjectType().
		WithProperty("buffer", types.Any). // Reference to underlying ArrayBuffer
		WithProperty("byteLength", types.Number).
		WithProperty("byteOffset", types.Number).
		WithProperty("length", types.Number).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("set", types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Undefined)).
		WithProperty("subarray", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true})).
		WithProperty("slice", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true}))

	// Create Float64Array constructor type with multiple overloads
	float64ArrayCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Number}, float64ArrayProtoType).                                // Float64Array(length)
		WithSimpleCallSignature([]types.Type{types.Any}, float64ArrayProtoType).                                   // Float64Array(buffer, byteOffset?, length?)
		WithSimpleCallSignature([]types.Type{&types.ArrayType{ElementType: types.Number}}, float64ArrayProtoType). // Float64Array(array)
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("from", types.NewSimpleFunction([]types.Type{types.Any}, float64ArrayProtoType)).
		WithProperty("of", types.NewSimpleFunction([]types.Type{}, float64ArrayProtoType)).
		WithProperty("prototype", float64ArrayProtoType)

	return ctx.DefineGlobal("Float64Array", float64ArrayCtorType)
}

func (u *Float64ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Float64Array.prototype inheriting from TypedArray.prototype
	float64ArrayProto := vm.NewObject(vmInstance.TypedArrayPrototype).AsPlainObject()

	// Add prototype properties
	float64ArrayProto.SetOwnNonEnumerable("BYTES_PER_ELEMENT", vm.Number(8))

	// Add buffer getter
	float64ArrayProto.SetOwnNonEnumerable("buffer", vm.NewNativeFunction(0, false, "get buffer", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if ta := thisArray.AsTypedArray(); ta != nil {
			return vm.Value{}, nil // TODO: Return proper ArrayBuffer value
		}
		return vm.Undefined, nil
	}))

	// Add byteLength getter
	float64ArrayProto.SetOwnNonEnumerable("byteLength", vm.NewNativeFunction(0, false, "get byteLength", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if ta := thisArray.AsTypedArray(); ta != nil {
			return vm.Number(float64(ta.GetByteLength())), nil
		}
		return vm.Undefined, nil
	}))

	// Add byteOffset getter
	float64ArrayProto.SetOwnNonEnumerable("byteOffset", vm.NewNativeFunction(0, false, "get byteOffset", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if ta := thisArray.AsTypedArray(); ta != nil {
			return vm.Number(float64(ta.GetByteOffset())), nil
		}
		return vm.Undefined, nil
	}))

	// Add length getter
	float64ArrayProto.SetOwnNonEnumerable("length", vm.NewNativeFunction(0, false, "get length", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if ta := thisArray.AsTypedArray(); ta != nil {
			return vm.Number(float64(ta.GetLength())), nil
		}
		return vm.Undefined, nil
	}))

	// Add set method
	float64ArrayProto.SetOwnNonEnumerable("set", vm.NewNativeFunction(2, false, "set", func(args []vm.Value) (vm.Value, error) {
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
	float64ArrayProto.SetOwnNonEnumerable("fill", vm.NewNativeFunction(3, false, "fill", func(args []vm.Value) (vm.Value, error) {
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
	float64ArrayProto.SetOwnNonEnumerable("subarray", vm.NewNativeFunction(2, false, "subarray", func(args []vm.Value) (vm.Value, error) {
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

		// Create new view into same buffer - byte offset must be aligned for Float64
		byteStart := ta.GetByteOffset() + start*8
		length := end - start
		return vm.NewTypedArray(vm.TypedArrayFloat64, ta.GetBuffer(), byteStart, length), nil
	}))

	// Add slice method (creates new array)
	float64ArrayProto.SetOwnNonEnumerable("slice", vm.NewNativeFunction(2, false, "slice", func(args []vm.Value) (vm.Value, error) {
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
		newArray := vm.NewTypedArray(vm.TypedArrayFloat64, length, 0, 0)
		if newTA := newArray.AsTypedArray(); newTA != nil {
			for i := 0; i < length; i++ {
				newTA.SetElement(i, ta.GetElement(start+i))
			}
		}

		return newArray, nil
	}))

	// Create Float64Array constructor
	ctorWithProps := vm.NewConstructorWithProps(-1, true, "Float64Array", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayFloat64, 0, 0, 0), nil
		}

		arg := args[0]

		// Handle different constructor patterns
		if arg.IsNumber() {
			// Float64Array(length)
			length := int(arg.ToFloat())
			if length < 0 {
				// Should throw RangeError
				return vm.Undefined, nil
			}
			return vm.NewTypedArray(vm.TypedArrayFloat64, length, 0, 0), nil
		}

		if buffer := arg.AsArrayBuffer(); buffer != nil {
			// Float64Array(buffer, byteOffset?, length?)
			byteOffset := 0
			if len(args) > 1 {
				byteOffset = int(args[1].ToFloat())
			}

			length := -1 // Use remaining buffer
			if len(args) > 2 {
				length = int(args[2].ToFloat())
			}

			return vm.NewTypedArray(vm.TypedArrayFloat64, buffer, byteOffset, length), nil
		}

		if sourceArray := arg.AsArray(); sourceArray != nil {
			// Float64Array(array)
			values := make([]vm.Value, sourceArray.Length())
			for i := 0; i < sourceArray.Length(); i++ {
				values[i] = sourceArray.Get(i)
			}
			return vm.NewTypedArray(vm.TypedArrayFloat64, values, 0, 0), nil
		}

		// Default case
		return vm.NewTypedArray(vm.TypedArrayFloat64, 0, 0, 0), nil
	})

	// Add prototype property
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(float64ArrayProto))

	// Add static properties and methods
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("BYTES_PER_ELEMENT", vm.Number(8))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayFloat64, 0, 0, 0), nil
		}

		source := args[0]
		if sourceArray := source.AsArray(); sourceArray != nil {
			values := make([]vm.Value, sourceArray.Length())
			for i := 0; i < sourceArray.Length(); i++ {
				values[i] = sourceArray.Get(i)
			}
			return vm.NewTypedArray(vm.TypedArrayFloat64, values, 0, 0), nil
		}

		return vm.NewTypedArray(vm.TypedArrayFloat64, 0, 0, 0), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		return vm.NewTypedArray(vm.TypedArrayFloat64, args, 0, 0), nil
	}))

	// Set constructor property on prototype
	float64ArrayProto.SetOwnNonEnumerable("constructor", ctorWithProps)

	// Set Float64Array prototype in VM
	vmInstance.Float64ArrayPrototype = vm.NewValueFromPlainObject(float64ArrayProto)

	// Register Float64Array constructor as global
	return ctx.DefineGlobal("Float64Array", ctorWithProps)
}
