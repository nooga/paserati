package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type Int32ArrayInitializer struct{}

func (i *Int32ArrayInitializer) Name() string {
	return "Int32Array"
}

func (i *Int32ArrayInitializer) Priority() int {
	return 421 // After Uint8Array
}

func (i *Int32ArrayInitializer) InitTypes(ctx *TypeContext) error {
	// Create Int32Array.prototype type
	int32ArrayProtoType := types.NewObjectType().
		WithProperty("buffer", types.Any). // Reference to underlying ArrayBuffer
		WithProperty("byteLength", types.Number).
		WithProperty("byteOffset", types.Number).
		WithProperty("length", types.Number).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("set", types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Undefined)).
		WithProperty("subarray", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true})).
		WithProperty("slice", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true}))

	// Create Int32Array constructor type with multiple overloads
	int32ArrayCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Number}, int32ArrayProtoType).                                // Int32Array(length)
		WithSimpleCallSignature([]types.Type{types.Any}, int32ArrayProtoType).                                   // Int32Array(buffer, byteOffset?, length?)
		WithSimpleCallSignature([]types.Type{&types.ArrayType{ElementType: types.Number}}, int32ArrayProtoType). // Int32Array(array)
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("from", types.NewSimpleFunction([]types.Type{types.Any}, int32ArrayProtoType)).
		WithProperty("of", types.NewSimpleFunction([]types.Type{}, int32ArrayProtoType)).
		WithProperty("prototype", int32ArrayProtoType)

	return ctx.DefineGlobal("Int32Array", int32ArrayCtorType)
}

func (i *Int32ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Int32Array.prototype inheriting from TypedArray.prototype
	int32ArrayProto := vm.NewObject(vmInstance.TypedArrayPrototype).AsPlainObject()

	// Set up prototype properties with correct descriptors (BYTES_PER_ELEMENT, buffer, byteLength, byteOffset, length)
	SetupTypedArrayPrototypeProperties(int32ArrayProto, vmInstance, 4)
	// Note: set, fill, subarray, slice, and Symbol.toStringTag are inherited from %TypedArray%.prototype

	// Create Int32Array constructor
	// Create Int32Array constructor (length is 3 per ECMAScript spec)
	ctorWithProps := vm.NewConstructorWithProps(3, true, "Int32Array", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayInt32, 0, 0, 0), nil
		}

		arg := args[0]

		// Handle different constructor patterns
		if arg.IsNumber() {
			// Int32Array(length)
			length := int(arg.ToFloat())
			if length < 0 {
				// Should throw RangeError
				return vm.Undefined, nil
			}
			return vm.NewTypedArray(vm.TypedArrayInt32, length, 0, 0), nil
		}

		if buffer := arg.AsArrayBuffer(); buffer != nil {
			// Int32Array(buffer, byteOffset?, length?)
			byteOffset := 0
			if len(args) > 1 {
				var err error
				byteOffset, err = ValidateTypedArrayByteOffset(vmInstance, args[1], 4)
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
				if err := ValidateTypedArrayBufferAlignment(vmInstance, buffer, byteOffset, 4); err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}

			return vm.NewTypedArray(vm.TypedArrayInt32, buffer, byteOffset, length), nil
		}

		if sharedBuffer := arg.AsSharedArrayBuffer(); sharedBuffer != nil {
			// Int32Array(sharedBuffer, byteOffset?, length?)
			byteOffset := 0
			if len(args) > 1 {
				var err error
				byteOffset, err = ValidateTypedArrayByteOffsetShared(vmInstance, args[1], 4)
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
				if err := ValidateTypedArrayBufferAlignmentShared(vmInstance, sharedBuffer, byteOffset, 4); err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}

			return vm.NewTypedArray(vm.TypedArrayInt32, sharedBuffer, byteOffset, length), nil
		}

		if sourceArray := arg.AsArray(); sourceArray != nil {
			// Int32Array(array)
			values := make([]vm.Value, sourceArray.Length())
			for i := 0; i < sourceArray.Length(); i++ {
				values[i] = sourceArray.Get(i)
			}
			return vm.NewTypedArray(vm.TypedArrayInt32, values, 0, 0), nil
		}

		// Default case
		return vm.NewTypedArray(vm.TypedArrayInt32, 0, 0, 0), nil
	})

	// Set up constructor properties with correct descriptors (BYTES_PER_ELEMENT, prototype)
	SetupTypedArrayConstructorProperties(ctorWithProps, int32ArrayProto, 4)

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayInt32, 0, 0, 0), nil
		}

		source := args[0]
		if sourceArray := source.AsArray(); sourceArray != nil {
			values := make([]vm.Value, sourceArray.Length())
			for i := 0; i < sourceArray.Length(); i++ {
				values[i] = sourceArray.Get(i)
			}
			return vm.NewTypedArray(vm.TypedArrayInt32, values, 0, 0), nil
		}

		return vm.NewTypedArray(vm.TypedArrayInt32, 0, 0, 0), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		return vm.NewTypedArray(vm.TypedArrayInt32, args, 0, 0), nil
	}))

	// Set constructor property on prototype
	int32ArrayProto.SetOwnNonEnumerable("constructor", ctorWithProps)

	// Set the constructor's [[Prototype]] to TypedArray (for proper inheritance chain)
	// This makes Object.getPrototypeOf(Int32Array) === TypedArray
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetPrototype(vmInstance.TypedArrayConstructor)

	// Set Int32Array prototype in VM
	vmInstance.Int32ArrayPrototype = vm.NewValueFromPlainObject(int32ArrayProto)

	// Register Int32Array constructor as global
	return ctx.DefineGlobal("Int32Array", ctorWithProps)
}
