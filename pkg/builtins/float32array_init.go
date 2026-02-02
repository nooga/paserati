package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
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

	// Create Float32Array.prototype inheriting from TypedArray.prototype
	float32ArrayProto := vm.NewObject(vmInstance.TypedArrayPrototype).AsPlainObject()

	// Set up prototype properties with correct descriptors (BYTES_PER_ELEMENT, buffer, byteLength, byteOffset, length)
	SetupTypedArrayPrototypeProperties(float32ArrayProto, vmInstance, 4)
	// Note: set, fill, subarray, slice, and Symbol.toStringTag are inherited from %TypedArray%.prototype

	// constructor (length is 3 per ECMAScript spec)
	ctorWithProps := vm.NewConstructorWithProps(3, true, "Float32Array", func(args []vm.Value) (vm.Value, error) {
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

			return vm.NewTypedArray(vm.TypedArrayFloat32, buffer, byteOffset, length), nil
		}

		if sab := arg.AsSharedArrayBuffer(); sab != nil {
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
			length := -1
			if len(args) > 2 && !args[2].IsUndefined() {
				length = int(args[2].ToFloat())
			}
			if length == -1 {
				if err := ValidateTypedArrayBufferAlignmentShared(vmInstance, sab, byteOffset, 4); err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}
			return vm.NewTypedArray(vm.TypedArrayFloat32, sab, byteOffset, length), nil
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

	// Set up constructor properties with correct descriptors (BYTES_PER_ELEMENT, prototype)
	SetupTypedArrayConstructorProperties(ctorWithProps, float32ArrayProto, 4)

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

	// Set the constructor's [[Prototype]] to TypedArray (for proper inheritance chain)
	// This makes Object.getPrototypeOf(Float32Array) === TypedArray
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetPrototype(vmInstance.TypedArrayConstructor)

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
