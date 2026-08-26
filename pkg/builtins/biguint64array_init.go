package builtins

import (
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
	bigUint64ArrayProtoType := typedArrayInstanceType(types.BigInt)

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
	bigUint64EK := TypedArrayElementKind{Kind: vm.TypedArrayBigUint64, ElementSize: 8, IsBigInt: true, Unsigned: true}
	ctorWithProps := vm.NewConstructorWithProps(3, true, "BigUint64Array", NumericTypedArrayCtorBody(vmInstance, bigUint64EK))

	// Set up constructor properties with correct descriptors (BYTES_PER_ELEMENT, prototype)
	SetupTypedArrayConstructorProperties(ctorWithProps, bigUint64ArrayProto, 8)

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayFromArrayLike(bigUint64EK, args), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayOfValues(bigUint64EK, args), nil
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
