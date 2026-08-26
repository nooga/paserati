package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type Uint8ClampedArrayInitializer struct{}

func (u *Uint8ClampedArrayInitializer) Name() string {
	return "Uint8ClampedArray"
}

func (u *Uint8ClampedArrayInitializer) Priority() int {
	return 421 // After Uint8Array
}

func (u *Uint8ClampedArrayInitializer) InitTypes(ctx *TypeContext) error {
	// Create Uint8ClampedArray.prototype type
	uint8ClampedArrayProtoType := typedArrayInstanceType(types.Number)

	// Create Uint8ClampedArray constructor type with multiple overloads
	uint8ClampedArrayCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Number}, uint8ClampedArrayProtoType).                                // Uint8ClampedArray(length)
		WithSimpleCallSignature([]types.Type{types.Any}, uint8ClampedArrayProtoType).                                   // Uint8ClampedArray(buffer, byteOffset?, length?)
		WithSimpleCallSignature([]types.Type{&types.ArrayType{ElementType: types.Number}}, uint8ClampedArrayProtoType). // Uint8ClampedArray(array)
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("from", types.NewSimpleFunction([]types.Type{types.Any}, uint8ClampedArrayProtoType)).
		WithProperty("of", types.NewSimpleFunction([]types.Type{}, uint8ClampedArrayProtoType)).
		WithProperty("prototype", uint8ClampedArrayProtoType)

	return ctx.DefineGlobal("Uint8ClampedArray", uint8ClampedArrayCtorType)
}

func (u *Uint8ClampedArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Uint8ClampedArray.prototype inheriting from TypedArray.prototype
	uint8ClampedArrayProto := vm.NewObject(vmInstance.TypedArrayPrototype).AsPlainObject()

	// Set up prototype properties with correct descriptors (BYTES_PER_ELEMENT, buffer, byteLength, byteOffset, length)
	SetupTypedArrayPrototypeProperties(uint8ClampedArrayProto, vmInstance, 1)
	// Note: set, fill, subarray, slice, and Symbol.toStringTag are inherited from %TypedArray%.prototype

	// constructor (length is 3 per ECMAScript spec)
	uint8ClampedEK := TypedArrayElementKind{Kind: vm.TypedArrayUint8Clamped, ElementSize: 1}
	ctorWithProps := vm.NewConstructorWithProps(3, true, "Uint8ClampedArray", NumericTypedArrayCtorBody(vmInstance, uint8ClampedEK))

	// Set up constructor properties with correct descriptors (BYTES_PER_ELEMENT, prototype)
	SetupTypedArrayConstructorProperties(ctorWithProps, uint8ClampedArrayProto, 1)

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayFromArrayLike(uint8ClampedEK, args), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayOfValues(uint8ClampedEK, args), nil
	}))

	// Set constructor property on prototype
	uint8ClampedArrayProto.SetOwnNonEnumerable("constructor", ctorWithProps)

	// Set the constructor's [[Prototype]] to TypedArray (for proper inheritance chain)
	// This makes Object.getPrototypeOf(Uint8ClampedArray) === TypedArray
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetPrototype(vmInstance.TypedArrayConstructor)

	// Set Uint8ClampedArray prototype in VM
	vmInstance.Uint8ClampedArrayPrototype = vm.NewValueFromPlainObject(uint8ClampedArrayProto)

	// Register Uint8ClampedArray constructor as global
	return ctx.DefineGlobal("Uint8ClampedArray", ctorWithProps)
}
