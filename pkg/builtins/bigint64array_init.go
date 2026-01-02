package builtins

import (
	"math/big"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

type BigInt64ArrayInitializer struct{}

func (i *BigInt64ArrayInitializer) Name() string {
	return "BigInt64Array"
}

func (i *BigInt64ArrayInitializer) Priority() int {
	return 430 // After other TypedArrays
}

func (i *BigInt64ArrayInitializer) InitTypes(ctx *TypeContext) error {
	// Create BigInt64Array.prototype type
	bigInt64ArrayProtoType := types.NewObjectType().
		WithProperty("buffer", types.Any). // Reference to underlying ArrayBuffer
		WithProperty("byteLength", types.Number).
		WithProperty("byteOffset", types.Number).
		WithProperty("length", types.Number).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("set", types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Undefined)).
		WithProperty("subarray", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true})).
		WithProperty("slice", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true}))

	// Create BigInt64Array constructor type with multiple overloads
	bigInt64ArrayCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Number}, bigInt64ArrayProtoType).                                // BigInt64Array(length)
		WithSimpleCallSignature([]types.Type{types.Any}, bigInt64ArrayProtoType).                                   // BigInt64Array(buffer, byteOffset?, length?)
		WithSimpleCallSignature([]types.Type{&types.ArrayType{ElementType: types.BigInt}}, bigInt64ArrayProtoType). // BigInt64Array(array)
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("from", types.NewSimpleFunction([]types.Type{types.Any}, bigInt64ArrayProtoType)).
		WithProperty("of", types.NewSimpleFunction([]types.Type{}, bigInt64ArrayProtoType)).
		WithProperty("prototype", bigInt64ArrayProtoType)

	return ctx.DefineGlobal("BigInt64Array", bigInt64ArrayCtorType)
}

func (i *BigInt64ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create BigInt64Array.prototype inheriting from Object.prototype
	bigInt64ArrayProto := vm.NewObject(objectProto).AsPlainObject()

	// Add prototype properties
	bigInt64ArrayProto.SetOwnNonEnumerable("BYTES_PER_ELEMENT", vm.Number(8))

	// Add buffer getter
	bigInt64ArrayProto.SetOwnNonEnumerable("buffer", vm.NewNativeFunction(0, false, "get buffer", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if ta := thisArray.AsTypedArray(); ta != nil {
			return vm.Value{}, nil // TODO: Return proper ArrayBuffer value
		}
		return vm.Undefined, nil
	}))

	// Add byteLength getter
	bigInt64ArrayProto.SetOwnNonEnumerable("byteLength", vm.NewNativeFunction(0, false, "get byteLength", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if ta := thisArray.AsTypedArray(); ta != nil {
			return vm.Number(float64(ta.GetByteLength())), nil
		}
		return vm.Undefined, nil
	}))

	// Add byteOffset getter
	bigInt64ArrayProto.SetOwnNonEnumerable("byteOffset", vm.NewNativeFunction(0, false, "get byteOffset", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if ta := thisArray.AsTypedArray(); ta != nil {
			return vm.Number(float64(ta.GetByteOffset())), nil
		}
		return vm.Undefined, nil
	}))

	// Add length getter
	bigInt64ArrayProto.SetOwnNonEnumerable("length", vm.NewNativeFunction(0, false, "get length", func(args []vm.Value) (vm.Value, error) {
		thisArray := vmInstance.GetThis()
		if ta := thisArray.AsTypedArray(); ta != nil {
			return vm.Number(float64(ta.GetLength())), nil
		}
		return vm.Undefined, nil
	}))

	// Add set method
	bigInt64ArrayProto.SetOwnNonEnumerable("set", vm.NewNativeFunction(2, false, "set", func(args []vm.Value) (vm.Value, error) {
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
	bigInt64ArrayProto.SetOwnNonEnumerable("fill", vm.NewNativeFunction(3, false, "fill", func(args []vm.Value) (vm.Value, error) {
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
	bigInt64ArrayProto.SetOwnNonEnumerable("subarray", vm.NewNativeFunction(2, false, "subarray", func(args []vm.Value) (vm.Value, error) {
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
		byteStart := ta.GetByteOffset() + start*8 // 8 bytes per BigInt64 element
		length := end - start
		return vm.NewTypedArray(vm.TypedArrayBigInt64, ta.GetBuffer(), byteStart, length), nil
	}))

	// Add slice method (creates new array)
	bigInt64ArrayProto.SetOwnNonEnumerable("slice", vm.NewNativeFunction(2, false, "slice", func(args []vm.Value) (vm.Value, error) {
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
		newArray := vm.NewTypedArray(vm.TypedArrayBigInt64, length, 0, 0)
		if newTA := newArray.AsTypedArray(); newTA != nil {
			for i := 0; i < length; i++ {
				newTA.SetElement(i, ta.GetElement(start+i))
			}
		}

		return newArray, nil
	}))

	// Create BigInt64Array constructor
	ctorWithProps := vm.NewConstructorWithProps(-1, true, "BigInt64Array", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayBigInt64, 0, 0, 0), nil
		}

		arg := args[0]

		// Handle different constructor patterns
		if arg.IsNumber() {
			// BigInt64Array(length)
			length := int(arg.ToFloat())
			if length < 0 {
				// Should throw RangeError
				return vm.Undefined, nil
			}
			return vm.NewTypedArray(vm.TypedArrayBigInt64, length, 0, 0), nil
		}

		if buffer := arg.AsArrayBuffer(); buffer != nil {
			// BigInt64Array(buffer, byteOffset?, length?)
			byteOffset := 0
			if len(args) > 1 {
				byteOffset = int(args[1].ToFloat())
			}

			length := -1 // Use remaining buffer
			if len(args) > 2 {
				length = int(args[2].ToFloat())
			}

			return vm.NewTypedArray(vm.TypedArrayBigInt64, buffer, byteOffset, length), nil
		}

		if sourceArray := arg.AsArray(); sourceArray != nil {
			// BigInt64Array(array)
			values := make([]vm.Value, sourceArray.Length())
			for i := 0; i < sourceArray.Length(); i++ {
				v := sourceArray.Get(i)
				// Convert to BigInt if not already
				if !v.IsBigInt() {
					v = vm.NewBigInt(big.NewInt(int64(v.ToFloat())))
				}
				values[i] = v
			}
			return vm.NewTypedArray(vm.TypedArrayBigInt64, values, 0, 0), nil
		}

		// Default case
		return vm.NewTypedArray(vm.TypedArrayBigInt64, 0, 0, 0), nil
	})

	// Add prototype property
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(bigInt64ArrayProto))

	// Add static properties and methods
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("BYTES_PER_ELEMENT", vm.Number(8))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayBigInt64, 0, 0, 0), nil
		}

		source := args[0]
		if sourceArray := source.AsArray(); sourceArray != nil {
			values := make([]vm.Value, sourceArray.Length())
			for i := 0; i < sourceArray.Length(); i++ {
				v := sourceArray.Get(i)
				// Convert to BigInt if not already
				if !v.IsBigInt() {
					v = vm.NewBigInt(big.NewInt(int64(v.ToFloat())))
				}
				values[i] = v
			}
			return vm.NewTypedArray(vm.TypedArrayBigInt64, values, 0, 0), nil
		}

		return vm.NewTypedArray(vm.TypedArrayBigInt64, 0, 0, 0), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		values := make([]vm.Value, len(args))
		for i, v := range args {
			// Convert to BigInt if not already
			if !v.IsBigInt() {
				v = vm.NewBigInt(big.NewInt(int64(v.ToFloat())))
			}
			values[i] = v
		}
		return vm.NewTypedArray(vm.TypedArrayBigInt64, values, 0, 0), nil
	}))

	// Add common TypedArray prototype methods
	SetupTypedArrayPrototype(bigInt64ArrayProto, vmInstance)

	// Set constructor property on prototype
	bigInt64ArrayProto.SetOwnNonEnumerable("constructor", ctorWithProps)

	// Set BigInt64Array prototype in VM
	vmInstance.BigInt64ArrayPrototype = vm.NewValueFromPlainObject(bigInt64ArrayProto)

	// Register BigInt64Array constructor as global
	return ctx.DefineGlobal("BigInt64Array", ctorWithProps)
}
