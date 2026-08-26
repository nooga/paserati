package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type Int16ArrayInitializer struct{}

func (i *Int16ArrayInitializer) Name() string  { return "Int16Array" }
func (i *Int16ArrayInitializer) Priority() int { return 422 }

func (i *Int16ArrayInitializer) InitTypes(ctx *TypeContext) error {
	proto := typedArrayInstanceType(types.Number)

	ctor := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Number}, proto).
		WithSimpleCallSignature([]types.Type{types.Any}, proto).
		WithSimpleCallSignature([]types.Type{&types.ArrayType{ElementType: types.Number}}, proto).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("from", types.NewSimpleFunction([]types.Type{types.Any}, proto)).
		WithProperty("of", types.NewSimpleFunction([]types.Type{}, proto)).
		WithProperty("prototype", proto)

	return ctx.DefineGlobal("Int16Array", ctor)
}

func (i *Int16ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmx := ctx.VM
	proto := vm.NewObject(vmx.TypedArrayPrototype).AsPlainObject()

	// Set up prototype properties with correct descriptors (BYTES_PER_ELEMENT, buffer, byteLength, byteOffset, length)
	SetupTypedArrayPrototypeProperties(proto, vmx, 2)
	// Note: set, fill, subarray, slice, and Symbol.toStringTag are inherited from %TypedArray%.prototype

	// constructor (length is 3 per ECMAScript spec)
	int16EK := TypedArrayElementKind{Kind: vm.TypedArrayInt16, ElementSize: 2}
	ctor := vm.NewConstructorWithProps(3, true, "Int16Array", NumericTypedArrayCtorBody(vmx, int16EK))
	// Set up constructor properties with correct descriptors (BYTES_PER_ELEMENT, prototype)
	SetupTypedArrayConstructorProperties(ctor, proto, 2)

	ctor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayFromArrayLike(int16EK, args), nil
	}))

	ctor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayOfValues(int16EK, args), nil
	}))

	// Set constructor property on prototype
	proto.SetOwnNonEnumerable("constructor", ctor)

	// Set the constructor's [[Prototype]] to TypedArray (for proper inheritance chain)
	// This makes Object.getPrototypeOf(Int16Array) === TypedArray
	ctor.AsNativeFunctionWithProps().Properties.SetPrototype(vmx.TypedArrayConstructor)

	vmx.Int16ArrayPrototype = vm.NewValueFromPlainObject(proto)
	return ctx.DefineGlobal("Int16Array", ctor)
}
