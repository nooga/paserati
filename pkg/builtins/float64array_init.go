package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
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

	// Set up prototype properties with correct descriptors (BYTES_PER_ELEMENT, buffer, byteLength, byteOffset, length)
	SetupTypedArrayPrototypeProperties(float64ArrayProto, vmInstance, 8)
	// Note: set, fill, subarray, slice, and Symbol.toStringTag are inherited from %TypedArray%.prototype

	// constructor (length is 3 per ECMAScript spec)
	ctorWithProps := vm.NewConstructorWithProps(3, true, "Float64Array", func(args []vm.Value) (vm.Value, error) {
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

			return vm.NewTypedArray(vm.TypedArrayFloat64, buffer, byteOffset, length), nil
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
			return vm.NewTypedArray(vm.TypedArrayFloat64, sab, byteOffset, length), nil
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

	// Set up constructor properties with correct descriptors (BYTES_PER_ELEMENT, prototype)
	SetupTypedArrayConstructorProperties(ctorWithProps, float64ArrayProto, 8)

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

	// Set the constructor's [[Prototype]] to TypedArray (for proper inheritance chain)
	// This makes Object.getPrototypeOf(Float64Array) === TypedArray
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetPrototype(vmInstance.TypedArrayConstructor)

	// Set Float64Array prototype in VM
	vmInstance.Float64ArrayPrototype = vm.NewValueFromPlainObject(float64ArrayProto)

	// Register Float64Array constructor as global
	return ctx.DefineGlobal("Float64Array", ctorWithProps)
}
