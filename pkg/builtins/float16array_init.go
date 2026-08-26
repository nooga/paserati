package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// Float16ArrayInitializer registers Float16Array (ES2024), the half-precision
// (IEEE 754 binary16) typed array. Its element storage/conversion lives in
// pkg/vm/typed_array.go (vm.Float64ToFloat16Bits/vm.Float16BitsToFloat64,
// shared with Math.f16round and DataView.prototype.{get,set}Float16).
type Float16ArrayInitializer struct{}

func (f *Float16ArrayInitializer) Name() string  { return "Float16Array" }
func (f *Float16ArrayInitializer) Priority() int { return 424 } // After Float64Array

func (f *Float16ArrayInitializer) InitTypes(ctx *TypeContext) error {
	protoType := typedArrayInstanceType(types.Number)

	ctorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Number}, protoType).
		WithSimpleCallSignature([]types.Type{types.Any}, protoType).
		WithSimpleCallSignature([]types.Type{&types.ArrayType{ElementType: types.Number}}, protoType).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("from", types.NewSimpleFunction([]types.Type{types.Any}, protoType)).
		WithProperty("of", types.NewSimpleFunction([]types.Type{}, protoType)).
		WithProperty("prototype", protoType)

	return ctx.DefineGlobal("Float16Array", ctorType)
}

func (f *Float16ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM
	proto := vm.NewObject(vmInstance.TypedArrayPrototype).AsPlainObject()

	SetupTypedArrayPrototypeProperties(proto, vmInstance, 2)

	ek := TypedArrayElementKind{Kind: vm.TypedArrayFloat16, ElementSize: 2}
	ctor := vm.NewConstructorWithProps(3, true, "Float16Array", NumericTypedArrayCtorBody(vmInstance, ek))
	SetupTypedArrayConstructorProperties(ctor, proto, 2)

	ctor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayFromArrayLike(ek, args), nil
	}))
	ctor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		return TypedArrayOfValues(ek, args), nil
	}))

	proto.SetOwnNonEnumerable("constructor", ctor)
	ctor.AsNativeFunctionWithProps().Properties.SetPrototype(vmInstance.TypedArrayConstructor)

	vmInstance.Float16ArrayPrototype = vm.NewValueFromPlainObject(proto)
	return ctx.DefineGlobal("Float16Array", ctor)
}
