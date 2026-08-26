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
	uint16ArrayProtoType := typedArrayInstanceType(types.Number)

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
	uint16EK := TypedArrayElementKind{Kind: vm.TypedArrayUint16, ElementSize: 2}
	ctorWithProps := vm.NewConstructorWithProps(3, true, "Uint16Array", NumericTypedArrayCtorBody(vmInstance, uint16EK))

	// Set up constructor properties with correct descriptors (BYTES_PER_ELEMENT, prototype)
	SetupTypedArrayConstructorProperties(ctorWithProps, uint16ArrayProto, 2)

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayFromArrayLike(uint16EK, args), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayOfValues(uint16EK, args), nil
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
