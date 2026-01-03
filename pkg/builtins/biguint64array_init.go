package builtins

import (
	"math/big"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

type BigUint64ArrayInitializer struct{}

func (i *BigUint64ArrayInitializer) Name() string {
	return "BigUint64Array"
}

func (i *BigUint64ArrayInitializer) Priority() int {
	return 431 // After BigInt64Array
}

func (i *BigUint64ArrayInitializer) InitTypes(ctx *TypeContext) error {
	// Create BigUint64Array.prototype type
	bigUint64ArrayProtoType := types.NewObjectType().
		WithProperty("buffer", types.Any). // Reference to underlying ArrayBuffer
		WithProperty("byteLength", types.Number).
		WithProperty("byteOffset", types.Number).
		WithProperty("length", types.Number).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("set", types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Undefined)).
		WithProperty("subarray", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true})).
		WithProperty("slice", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true}))

	// Create BigUint64Array constructor type with multiple overloads
	bigUint64ArrayCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Number}, bigUint64ArrayProtoType).                                // BigUint64Array(length)
		WithSimpleCallSignature([]types.Type{types.Any}, bigUint64ArrayProtoType).                                   // BigUint64Array(buffer, byteOffset?, length?)
		WithSimpleCallSignature([]types.Type{&types.ArrayType{ElementType: types.BigInt}}, bigUint64ArrayProtoType). // BigUint64Array(array)
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("from", types.NewSimpleFunction([]types.Type{types.Any}, bigUint64ArrayProtoType)).
		WithProperty("of", types.NewSimpleFunction([]types.Type{}, bigUint64ArrayProtoType)).
		WithProperty("prototype", bigUint64ArrayProtoType)

	return ctx.DefineGlobal("BigUint64Array", bigUint64ArrayCtorType)
}

func (i *BigUint64ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create BigUint64Array.prototype inheriting from TypedArray.prototype
	bigUint64ArrayProto := vm.NewObject(vmInstance.TypedArrayPrototype).AsPlainObject()

	// Add prototype properties
	bigUint64ArrayProto.SetOwnNonEnumerable("BYTES_PER_ELEMENT", vm.Number(8))

	// Add buffer getter
	bigUint64ArrayProto.SetOwnNonEnumerable("buffer", vm.NewNativeFunction(0, false, "get buffer", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if ta := thisArray.AsTypedArray(); ta != nil {
			return vm.Value{}, nil // TODO: Return proper ArrayBuffer value
		}
		return vm.Undefined, nil
	}))

	// Add byteLength getter
	bigUint64ArrayProto.SetOwnNonEnumerable("byteLength", vm.NewNativeFunction(0, false, "get byteLength", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if ta := thisArray.AsTypedArray(); ta != nil {
			return vm.Number(float64(ta.GetByteLength())), nil
		}
		return vm.Undefined, nil
	}))

	// Add byteOffset getter
	bigUint64ArrayProto.SetOwnNonEnumerable("byteOffset", vm.NewNativeFunction(0, false, "get byteOffset", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if ta := thisArray.AsTypedArray(); ta != nil {
			return vm.Number(float64(ta.GetByteOffset())), nil
		}
		return vm.Undefined, nil
	}))

	// Add length getter
	bigUint64ArrayProto.SetOwnNonEnumerable("length", vm.NewNativeFunction(0, false, "get length", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if ta := thisArray.AsTypedArray(); ta != nil {
			return vm.Number(float64(ta.GetLength())), nil
		}
		return vm.Undefined, nil
	}))

	// Add set method
	bigUint64ArrayProto.SetOwnNonEnumerable("set", vm.NewNativeFunction(2, false, "set", func(args []vm.Value) (vm.Value, error) {
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
	bigUint64ArrayProto.SetOwnNonEnumerable("fill", vm.NewNativeFunction(3, false, "fill", func(args []vm.Value) (vm.Value, error) {
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
	bigUint64ArrayProto.SetOwnNonEnumerable("subarray", vm.NewNativeFunction(2, false, "subarray", func(args []vm.Value) (vm.Value, error) {
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
		byteStart := ta.GetByteOffset() + start*8 // 8 bytes per BigUint64 element
		length := end - start
		return vm.NewTypedArray(vm.TypedArrayBigUint64, ta.GetBuffer(), byteStart, length), nil
	}))

	// Add slice method (creates new array)
	bigUint64ArrayProto.SetOwnNonEnumerable("slice", vm.NewNativeFunction(2, false, "slice", func(args []vm.Value) (vm.Value, error) {
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
		newArray := vm.NewTypedArray(vm.TypedArrayBigUint64, length, 0, 0)
		if newTA := newArray.AsTypedArray(); newTA != nil {
			for i := 0; i < length; i++ {
				newTA.SetElement(i, ta.GetElement(start+i))
			}
		}

		return newArray, nil
	}))

	// Create BigUint64Array constructor
	ctorWithProps := vm.NewConstructorWithProps(-1, true, "BigUint64Array", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayBigUint64, 0, 0, 0), nil
		}

		arg := args[0]

		// Handle different constructor patterns
		if arg.IsNumber() {
			// BigUint64Array(length)
			length := int(arg.ToFloat())
			if length < 0 {
				// Should throw RangeError
				return vm.Undefined, nil
			}
			return vm.NewTypedArray(vm.TypedArrayBigUint64, length, 0, 0), nil
		}

		if buffer := arg.AsArrayBuffer(); buffer != nil {
			// BigUint64Array(buffer, byteOffset?, length?)
			byteOffset := 0
			if len(args) > 1 {
				byteOffset = int(args[1].ToFloat())
			}

			length := -1 // Use remaining buffer
			if len(args) > 2 {
				length = int(args[2].ToFloat())
			}

			return vm.NewTypedArray(vm.TypedArrayBigUint64, buffer, byteOffset, length), nil
		}

		if sourceArray := arg.AsArray(); sourceArray != nil {
			// BigUint64Array(array)
			values := make([]vm.Value, sourceArray.Length())
			for i := 0; i < sourceArray.Length(); i++ {
				v := sourceArray.Get(i)
				// Convert to BigInt if not already
				if !v.IsBigInt() {
					v = vm.NewBigInt(new(big.Int).SetUint64(uint64(v.ToFloat())))
				}
				values[i] = v
			}
			return vm.NewTypedArray(vm.TypedArrayBigUint64, values, 0, 0), nil
		}

		// Default case
		return vm.NewTypedArray(vm.TypedArrayBigUint64, 0, 0, 0), nil
	})

	// Add prototype property
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(bigUint64ArrayProto))

	// Add static properties and methods
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("BYTES_PER_ELEMENT", vm.Number(8))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayBigUint64, 0, 0, 0), nil
		}

		source := args[0]
		if sourceArray := source.AsArray(); sourceArray != nil {
			values := make([]vm.Value, sourceArray.Length())
			for i := 0; i < sourceArray.Length(); i++ {
				v := sourceArray.Get(i)
				// Convert to BigInt if not already
				if !v.IsBigInt() {
					v = vm.NewBigInt(new(big.Int).SetUint64(uint64(v.ToFloat())))
				}
				values[i] = v
			}
			return vm.NewTypedArray(vm.TypedArrayBigUint64, values, 0, 0), nil
		}

		return vm.NewTypedArray(vm.TypedArrayBigUint64, 0, 0, 0), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		values := make([]vm.Value, len(args))
		for i, v := range args {
			// Convert to BigInt if not already
			if !v.IsBigInt() {
				v = vm.NewBigInt(new(big.Int).SetUint64(uint64(v.ToFloat())))
			}
			values[i] = v
		}
		return vm.NewTypedArray(vm.TypedArrayBigUint64, values, 0, 0), nil
	}))

	// Set constructor property on prototype
	bigUint64ArrayProto.SetOwnNonEnumerable("constructor", ctorWithProps)

	// Set BigUint64Array prototype in VM
	vmInstance.BigUint64ArrayPrototype = vm.NewValueFromPlainObject(bigUint64ArrayProto)

	// Register BigUint64Array constructor as global
	return ctx.DefineGlobal("BigUint64Array", ctorWithProps)
}
