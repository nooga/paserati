package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type Float64ArrayInitializer struct{}

func (u *Float64ArrayInitializer) Name() string {
	return "Float64Array"
}

func (u *Float64ArrayInitializer) Priority() int {
	return 423 // After Uint16Array
}

func (u *Float64ArrayInitializer) InitTypes(ctx *TypeContext) error {
	// Create Float64Array.prototype type
	float64ArrayProtoType := typedArrayInstanceType(types.Number)

	// Create Float64Array constructor type with multiple overloads
	float64ArrayCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Number}, float64ArrayProtoType).                                // Float64Array(length)
		WithSimpleCallSignature([]types.Type{types.Any}, float64ArrayProtoType).                                   // Float64Array(buffer, byteOffset?, length?)
		WithSimpleCallSignature([]types.Type{&types.ArrayType{ElementType: types.Number}}, float64ArrayProtoType). // Float64Array(array)
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("from", types.NewSimpleFunction([]types.Type{types.Any}, float64ArrayProtoType)).
		WithProperty("of", types.NewSimpleFunction([]types.Type{}, float64ArrayProtoType)).
		WithProperty("prototype", float64ArrayProtoType)

	return ctx.DefineGlobal("Float64Array", float64ArrayCtorType)
}

func (u *Float64ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Float64Array.prototype inheriting from TypedArray.prototype
	float64ArrayProto := vm.NewObject(vmInstance.TypedArrayPrototype).AsPlainObject()

	// Set up prototype properties with correct descriptors (BYTES_PER_ELEMENT, buffer, byteLength, byteOffset, length)
	SetupTypedArrayPrototypeProperties(float64ArrayProto, vmInstance, 8)
	// Note: set, fill, subarray, slice, and Symbol.toStringTag are inherited from %TypedArray%.prototype

	// constructor (length is 3 per ECMAScript spec)
	float64EK := TypedArrayElementKind{Kind: vm.TypedArrayFloat64, ElementSize: 8}
	ctorWithProps := vm.NewConstructorWithProps(3, true, "Float64Array", NumericTypedArrayCtorBody(vmInstance, float64EK))

	// Set up constructor properties with correct descriptors (BYTES_PER_ELEMENT, prototype)
	SetupTypedArrayConstructorProperties(ctorWithProps, float64ArrayProto, 8)

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayFromArrayLike(float64EK, args), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayOfValues(float64EK, args), nil
	}))

	// Set constructor property on prototype
	float64ArrayProto.SetOwnNonEnumerable("constructor", ctorWithProps)

	// Set the constructor's [[Prototype]] to TypedArray (for proper inheritance chain)
	// This makes Object.getPrototypeOf(Float64Array) === TypedArray
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetPrototype(vmInstance.TypedArrayConstructor)

	// Set Float64Array prototype in VM
	vmInstance.Float64ArrayPrototype = vm.NewValueFromPlainObject(float64ArrayProto)

	// Register Float64Array constructor as global
	return ctx.DefineGlobal("Float64Array", ctorWithProps)
}
