package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type Uint32ArrayInitializer struct{}

func (u *Uint32ArrayInitializer) Name() string  { return "Uint32Array" }
func (u *Uint32ArrayInitializer) Priority() int { return 423 }

func (u *Uint32ArrayInitializer) InitTypes(ctx *TypeContext) error {
	proto := typedArrayInstanceType(types.Number)

	ctor := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Number}, proto).
		WithSimpleCallSignature([]types.Type{types.Any}, proto).
		WithSimpleCallSignature([]types.Type{&types.ArrayType{ElementType: types.Number}}, proto).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("from", types.NewSimpleFunction([]types.Type{types.Any}, proto)).
		WithProperty("of", types.NewSimpleFunction([]types.Type{}, proto)).
		WithProperty("prototype", proto)

	return ctx.DefineGlobal("Uint32Array", ctor)
}

func (u *Uint32ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmx := ctx.VM
	proto := vm.NewObject(vmx.TypedArrayPrototype).AsPlainObject()

	// Set up prototype properties with correct descriptors (BYTES_PER_ELEMENT, buffer, byteLength, byteOffset, length)
	SetupTypedArrayPrototypeProperties(proto, vmx, 4)
	// Note: set, fill, subarray, slice, and Symbol.toStringTag are inherited from %TypedArray%.prototype

	// constructor (length is 3 per ECMAScript spec)
	uint32EK := TypedArrayElementKind{Kind: vm.TypedArrayUint32, ElementSize: 4}
	ctor := vm.NewConstructorWithProps(3, true, "Uint32Array", NumericTypedArrayCtorBody(vmx, uint32EK))
	// Set up constructor properties with correct descriptors (BYTES_PER_ELEMENT, prototype)
	SetupTypedArrayConstructorProperties(ctor, proto, 4)

	ctor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayFromArrayLike(uint32EK, args), nil
	}))

	ctor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayOfValues(uint32EK, args), nil
	}))

	// Set constructor property on prototype
	proto.SetOwnNonEnumerable("constructor", ctor)

	// Set the constructor's [[Prototype]] to TypedArray (for proper inheritance chain)
	// This makes Object.getPrototypeOf(Uint32Array) === TypedArray
	ctor.AsNativeFunctionWithProps().Properties.SetPrototype(vmx.TypedArrayConstructor)

	vmx.Uint32ArrayPrototype = vm.NewValueFromPlainObject(proto)
	return ctx.DefineGlobal("Uint32Array", ctor)
}
