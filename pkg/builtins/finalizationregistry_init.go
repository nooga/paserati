package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type FinalizationRegistryInitializer struct{}

func (f *FinalizationRegistryInitializer) Name() string {
	return "FinalizationRegistry"
}

func (f *FinalizationRegistryInitializer) Priority() int {
	return 416 // After WeakRef (415)
}

func (f *FinalizationRegistryInitializer) InitTypes(ctx *TypeContext) error {
	// Create generic type parameter T for FinalizationRegistry's held value.
	tParam := &types.TypeParameter{Name: "T", Index: 0}
	tType := &types.TypeParameterType{Parameter: tParam}

	finalizationRegistryType := &types.GenericType{
		Name:           "FinalizationRegistry",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           nil, // set below
	}

	instanceType := types.NewObjectType().
		WithProperty("register", types.NewOptionalFunction(
			[]types.Type{types.NewObjectType(), tType, types.Any},
			types.Undefined,
			[]bool{false, false, true},
		)).
		WithProperty("unregister", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean))

	finalizationRegistryType.Body = instanceType
	ctx.SetPrimitivePrototype("finalizationregistry", instanceType)

	ctorType := &types.GenericType{
		Name:           "FinalizationRegistry",
		TypeParameters: []*types.TypeParameter{tParam},
		Body: types.NewSimpleFunction(
			[]types.Type{types.NewSimpleFunction([]types.Type{tType}, types.Undefined)},
			finalizationRegistryType,
		),
	}

	if err := ctx.DefineGlobal("FinalizationRegistry", ctorType); err != nil {
		return err
	}
	return ctx.DefineTypeAlias("FinalizationRegistry", finalizationRegistryType)
}

func (f *FinalizationRegistryInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM
	objectProto := vmInstance.ObjectPrototype

	proto := vm.NewObject(objectProto).AsPlainObject()

	// FinalizationRegistry.prototype.register(target, heldValue, unregisterToken?)
	// Per https://tc39.es/ecma262/#sec-finalization-registry.prototype.register
	proto.SetOwnNonEnumerable("register", vm.NewNativeFunction(2, false, "register", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		fr := thisVal.AsFinalizationRegistry()
		if fr == nil {
			return vm.Undefined, vmInstance.NewTypeError("Method FinalizationRegistry.prototype.register called on incompatible receiver")
		}

		target := vm.Undefined
		if len(args) > 0 {
			target = args[0]
		}
		if !target.CanBeHeldWeakly() {
			return vm.Undefined, vmInstance.NewTypeError("target must be an object or a non-registered symbol")
		}

		heldValue := vm.Undefined
		if len(args) > 1 {
			heldValue = args[1]
		}
		if target.StrictlyEquals(heldValue) {
			return vm.Undefined, vmInstance.NewTypeError("target and heldValue must not be the same value")
		}

		hasToken := false
		var token vm.Value
		if len(args) > 2 && !args[2].IsUndefined() {
			token = args[2]
			if !token.CanBeHeldWeakly() {
				return vm.Undefined, vmInstance.NewTypeError("unregisterToken must be an object or a non-registered symbol")
			}
			hasToken = true
		}

		fr.Register(target, heldValue, hasToken, token)
		return vm.Undefined, nil
	}))
	if v, ok := proto.GetOwn("register"); ok {
		w, e, c := true, false, true
		proto.DefineOwnProperty("register", v, &w, &e, &c)
	}

	// FinalizationRegistry.prototype.unregister(unregisterToken)
	// Per https://tc39.es/ecma262/#sec-finalization-registry.prototype.unregister
	proto.SetOwnNonEnumerable("unregister", vm.NewNativeFunction(1, false, "unregister", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		fr := thisVal.AsFinalizationRegistry()
		if fr == nil {
			return vm.Undefined, vmInstance.NewTypeError("Method FinalizationRegistry.prototype.unregister called on incompatible receiver")
		}

		token := vm.Undefined
		if len(args) > 0 {
			token = args[0]
		}
		if !token.CanBeHeldWeakly() {
			return vm.Undefined, vmInstance.NewTypeError("unregisterToken must be an object or a non-registered symbol")
		}

		return vm.BooleanValue(fr.Unregister(token)), nil
	}))
	if v, ok := proto.GetOwn("unregister"); ok {
		w, e, c := true, false, true
		proto.DefineOwnProperty("unregister", v, &w, &e, &c)
	}

	// @@toStringTag: { writable: false, enumerable: false, configurable: true }
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		falseVal, trueVal := false, true
		proto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("FinalizationRegistry"),
			&falseVal, &falseVal, &trueVal,
		)
	}

	// Constructor: new FinalizationRegistry(cleanupCallback)
	// Per https://tc39.es/ecma262/#sec-finalization-registry-cleanupCallback
	ctor := vm.NewConstructorWithProps(1, false, "FinalizationRegistry", func(args []vm.Value) (vm.Value, error) {
		newTarget := vmInstance.GetNewTarget()
		if newTarget.IsUndefined() {
			return vm.Undefined, vmInstance.NewTypeError("Constructor FinalizationRegistry requires 'new'")
		}

		cleanupCallback := vm.Undefined
		if len(args) > 0 {
			cleanupCallback = args[0]
		}
		if !cleanupCallback.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("cleanupCallback must be callable")
		}

		prototype, err := vmInstance.GetPrototypeFromConstructor(newTarget, "%FinalizationRegistryPrototype%")
		if err != nil {
			return vm.Undefined, err
		}

		return vm.NewFinalizationRegistry(cleanupCallback, prototype), nil
	})

	w, e, c := false, false, false
	ctor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("prototype", vm.NewValueFromPlainObject(proto), &w, &e, &c)

	proto.SetOwnNonEnumerable("constructor", ctor)
	if v, ok := proto.GetOwn("constructor"); ok {
		w2, e2, c2 := true, false, true
		proto.DefineOwnProperty("constructor", v, &w2, &e2, &c2)
	}

	vmInstance.FinalizationRegistryPrototype = vm.NewValueFromPlainObject(proto)

	return ctx.DefineGlobal("FinalizationRegistry", ctor)
}
