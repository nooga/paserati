package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type Int8ArrayInitializer struct{}

func (i *Int8ArrayInitializer) Name() string  { return "Int8Array" }
func (i *Int8ArrayInitializer) Priority() int { return 421 }

func (i *Int8ArrayInitializer) InitTypes(ctx *TypeContext) error {
	int8ArrayProtoType := typedArrayInstanceType(types.Number)

	ctorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Number}, int8ArrayProtoType).
		WithSimpleCallSignature([]types.Type{types.Any}, int8ArrayProtoType).
		WithSimpleCallSignature([]types.Type{&types.ArrayType{ElementType: types.Number}}, int8ArrayProtoType).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("from", types.NewSimpleFunction([]types.Type{types.Any}, int8ArrayProtoType)).
		WithProperty("of", types.NewSimpleFunction([]types.Type{}, int8ArrayProtoType)).
		WithProperty("prototype", int8ArrayProtoType)

	return ctx.DefineGlobal("Int8Array", ctorType)
}

func (i *Int8ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmx := ctx.VM
	proto := vm.NewObject(vmx.TypedArrayPrototype).AsPlainObject()

	// Set up prototype properties with correct descriptors (BYTES_PER_ELEMENT, buffer, byteLength, byteOffset, length)
	SetupTypedArrayPrototypeProperties(proto, vmx, 1)
	// Note: set, fill, subarray, slice, and Symbol.toStringTag are inherited from %TypedArray%.prototype

	// constructor (length is 3 per ECMAScript spec)
	int8EK := TypedArrayElementKind{Kind: vm.TypedArrayInt8, ElementSize: 1}
	ctor := vm.NewConstructorWithProps(3, true, "Int8Array", NumericTypedArrayCtorBody(vmx, int8EK))
	// Set up constructor properties with correct descriptors (BYTES_PER_ELEMENT, prototype)
	SetupTypedArrayConstructorProperties(ctor, proto, 1)
	ctor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayFromArrayLike(int8EK, args), nil
	}))
	ctor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayOfValues(int8EK, args), nil
	}))

	proto.SetOwnNonEnumerable("constructor", ctor)

	// Set the constructor's [[Prototype]] to TypedArray (for proper inheritance chain)
	// This makes Object.getPrototypeOf(Int8Array) === TypedArray
	ctor.AsNativeFunctionWithProps().Properties.SetPrototype(vmx.TypedArrayConstructor)

	vmx.Int8ArrayPrototype = vm.NewValueFromPlainObject(proto)
	return ctx.DefineGlobal("Int8Array", ctor)
}
