package builtins

import (
	"math/big"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
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

	// Set up prototype properties with correct descriptors (BYTES_PER_ELEMENT, buffer, byteLength, byteOffset, length)
	SetupTypedArrayPrototypeProperties(bigUint64ArrayProto, vmInstance, 8)
	// Note: set, fill, subarray, slice, and Symbol.toStringTag are inherited from %TypedArray%.prototype

	// constructor (length is 3 per ECMAScript spec)
	ctorWithProps := vm.NewConstructorWithProps(3, true, "BigUint64Array", func(args []vm.Value) (vm.Value, error) {
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
				var err error
				byteOffset, err = ValidateTypedArrayByteOffset(vmInstance, args[1], 8)
				if err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}

			length := -1 // Use remaining buffer
			if len(args) > 2 && !args[2].IsUndefined() {
				length = int(args[2].ToFloat())
			}

			// If length is auto-calculated, validate buffer alignment
			if length == -1 {
				if err := ValidateTypedArrayBufferAlignment(vmInstance, buffer, byteOffset, 8); err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}

			return vm.NewTypedArray(vm.TypedArrayBigUint64, buffer, byteOffset, length), nil
		}

		if sab := arg.AsSharedArrayBuffer(); sab != nil {
			byteOffset := 0
			if len(args) > 1 {
				var err error
				byteOffset, err = ValidateTypedArrayByteOffsetShared(vmInstance, args[1], 8)
				if err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}
			length := -1
			if len(args) > 2 && !args[2].IsUndefined() {
				length = int(args[2].ToFloat())
			}
			if length == -1 {
				if err := ValidateTypedArrayBufferAlignmentShared(vmInstance, sab, byteOffset, 8); err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}
			return vm.NewTypedArray(vm.TypedArrayBigUint64, sab, byteOffset, length), nil
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

	// Set up constructor properties with correct descriptors (BYTES_PER_ELEMENT, prototype)
	SetupTypedArrayConstructorProperties(ctorWithProps, bigUint64ArrayProto, 8)

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

	// Set the constructor's [[Prototype]] to TypedArray (for proper inheritance chain)
	// This makes Object.getPrototypeOf(BigUint64Array) === TypedArray
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetPrototype(vmInstance.TypedArrayConstructor)

	// Set BigUint64Array prototype in VM
	vmInstance.BigUint64ArrayPrototype = vm.NewValueFromPlainObject(bigUint64ArrayProto)

	// Register BigUint64Array constructor as global
	return ctx.DefineGlobal("BigUint64Array", ctorWithProps)
}
