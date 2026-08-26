package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
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
	bigInt64ArrayProtoType := typedArrayInstanceType(types.BigInt)

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

	// Create BigInt64Array.prototype inheriting from TypedArray.prototype
	bigInt64ArrayProto := vm.NewObject(vmInstance.TypedArrayPrototype).AsPlainObject()

	// Set up prototype properties with correct descriptors (BYTES_PER_ELEMENT, buffer, byteLength, byteOffset, length)
	SetupTypedArrayPrototypeProperties(bigInt64ArrayProto, vmInstance, 8)
	// Note: set, fill, subarray, slice, and Symbol.toStringTag are inherited from %TypedArray%.prototype

	// constructor (length is 3 per ECMAScript spec)
	bigInt64EK := TypedArrayElementKind{Kind: vm.TypedArrayBigInt64, ElementSize: 8, IsBigInt: true}
	ctorWithProps := vm.NewConstructorWithProps(3, true, "BigInt64Array", NumericTypedArrayCtorBody(vmInstance, bigInt64EK))

	// Set up constructor properties with correct descriptors (BYTES_PER_ELEMENT, prototype)
	SetupTypedArrayConstructorProperties(ctorWithProps, bigInt64ArrayProto, 8)

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayFromArrayLike(bigInt64EK, args), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayOfValues(bigInt64EK, args), nil
	}))

	// Set constructor property on prototype
	bigInt64ArrayProto.SetOwnNonEnumerable("constructor", ctorWithProps)

	// Set the constructor's [[Prototype]] to TypedArray (for proper inheritance chain)
	// This makes Object.getPrototypeOf(BigInt64Array) === TypedArray
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetPrototype(vmInstance.TypedArrayConstructor)

	// Set BigInt64Array prototype in VM
	vmInstance.BigInt64ArrayPrototype = vm.NewValueFromPlainObject(bigInt64ArrayProto)

	// Register BigInt64Array constructor as global
	return ctx.DefineGlobal("BigInt64Array", ctorWithProps)
}
