package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type Uint16ArrayInitializer struct{}

func (u *Uint16ArrayInitializer) Name() string {
	return "Uint16Array"
}

func (u *Uint16ArrayInitializer) Priority() int {
	return 422 // After Uint8ClampedArray
}

func (u *Uint16ArrayInitializer) InitTypes(ctx *TypeContext) error {
	// Create Uint16Array.prototype type
	uint16ArrayProtoType := types.NewObjectType().
		WithProperty("buffer", types.Any). // Reference to underlying ArrayBuffer
		WithProperty("byteLength", types.Number).
		WithProperty("byteOffset", types.Number).
		WithProperty("length", types.Number).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("set", types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Undefined)).
		WithProperty("subarray", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true})).
		WithProperty("slice", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true}))

	// Create Uint16Array constructor type with multiple overloads
	uint16ArrayCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Number}, uint16ArrayProtoType).                                // Uint16Array(length)
		WithSimpleCallSignature([]types.Type{types.Any}, uint16ArrayProtoType).                                   // Uint16Array(buffer, byteOffset?, length?)
		WithSimpleCallSignature([]types.Type{&types.ArrayType{ElementType: types.Number}}, uint16ArrayProtoType). // Uint16Array(array)
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("from", types.NewSimpleFunction([]types.Type{types.Any}, uint16ArrayProtoType)).
		WithProperty("of", types.NewSimpleFunction([]types.Type{}, uint16ArrayProtoType)).
		WithProperty("prototype", uint16ArrayProtoType)

	return ctx.DefineGlobal("Uint16Array", uint16ArrayCtorType)
}

func (u *Uint16ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Uint16Array.prototype inheriting from TypedArray.prototype
	uint16ArrayProto := vm.NewObject(vmInstance.TypedArrayPrototype).AsPlainObject()

	// Set up prototype properties with correct descriptors (BYTES_PER_ELEMENT, buffer, byteLength, byteOffset, length)
	SetupTypedArrayPrototypeProperties(uint16ArrayProto, vmInstance, 2)
	// Note: set, fill, subarray, slice, and Symbol.toStringTag are inherited from %TypedArray%.prototype

	// constructor (length is 3 per ECMAScript spec)
	ctorWithProps := vm.NewConstructorWithProps(3, true, "Uint16Array", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayUint16, 0, 0, 0), nil
		}

		arg := args[0]

		// Handle different constructor patterns
		if arg.IsNumber() {
			// Uint16Array(length)
			length := int(arg.ToFloat())
			if length < 0 {
				// Should throw RangeError
				return vm.Undefined, nil
			}
			return vm.NewTypedArray(vm.TypedArrayUint16, length, 0, 0), nil
		}

		if buffer := arg.AsArrayBuffer(); buffer != nil {
			// Uint16Array(buffer, byteOffset?, length?)
			byteOffset := 0
			if len(args) > 1 {
				var err error
				byteOffset, err = ValidateTypedArrayByteOffset(vmInstance, args[1], 2)
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
				if err := ValidateTypedArrayBufferAlignment(vmInstance, buffer, byteOffset, 2); err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}

			return vm.NewTypedArray(vm.TypedArrayUint16, buffer, byteOffset, length), nil
		}

		if sab := arg.AsSharedArrayBuffer(); sab != nil {
			byteOffset := 0
			if len(args) > 1 {
				var err error
				byteOffset, err = ValidateTypedArrayByteOffsetShared(vmInstance, args[1], 2)
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
				if err := ValidateTypedArrayBufferAlignmentShared(vmInstance, sab, byteOffset, 2); err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}
			return vm.NewTypedArray(vm.TypedArrayUint16, sab, byteOffset, length), nil
		}

		if sourceArray := arg.AsArray(); sourceArray != nil {
			// Uint16Array(array)
			values := make([]vm.Value, sourceArray.Length())
			for i := 0; i < sourceArray.Length(); i++ {
				values[i] = sourceArray.Get(i)
			}
			return vm.NewTypedArray(vm.TypedArrayUint16, values, 0, 0), nil
		}

		// Default case
		return vm.NewTypedArray(vm.TypedArrayUint16, 0, 0, 0), nil
	})

	// Set up constructor properties with correct descriptors (BYTES_PER_ELEMENT, prototype)
	SetupTypedArrayConstructorProperties(ctorWithProps, uint16ArrayProto, 2)

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayUint16, 0, 0, 0), nil
		}

		source := args[0]
		if sourceArray := source.AsArray(); sourceArray != nil {
			values := make([]vm.Value, sourceArray.Length())
			for i := 0; i < sourceArray.Length(); i++ {
				values[i] = sourceArray.Get(i)
			}
			return vm.NewTypedArray(vm.TypedArrayUint16, values, 0, 0), nil
		}

		return vm.NewTypedArray(vm.TypedArrayUint16, 0, 0, 0), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		return vm.NewTypedArray(vm.TypedArrayUint16, args, 0, 0), nil
	}))

	// Set constructor property on prototype
	uint16ArrayProto.SetOwnNonEnumerable("constructor", ctorWithProps)

	// Set the constructor's [[Prototype]] to TypedArray (for proper inheritance chain)
	// This makes Object.getPrototypeOf(Uint16Array) === TypedArray
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetPrototype(vmInstance.TypedArrayConstructor)

	// Set Uint16Array prototype in VM
	vmInstance.Uint16ArrayPrototype = vm.NewValueFromPlainObject(uint16ArrayProto)

	// Register Uint16Array constructor as global
	return ctx.DefineGlobal("Uint16Array", ctorWithProps)
}
