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
	int32ArrayProtoType := typedArrayInstanceType(types.Number)

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

	// Create Int32Array constructor (length is 3 per ECMAScript spec)
	int32EK := TypedArrayElementKind{Kind: vm.TypedArrayInt32, ElementSize: 4}
	ctorWithProps := vm.NewConstructorWithProps(3, true, "Int32Array", NumericTypedArrayCtorBody(vmInstance, int32EK))

	// Set up constructor properties with correct descriptors (BYTES_PER_ELEMENT, prototype)
	SetupTypedArrayConstructorProperties(ctorWithProps, int32ArrayProto, 4)

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayFromArrayLike(int32EK, args), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayOfValues(int32EK, args), nil
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
