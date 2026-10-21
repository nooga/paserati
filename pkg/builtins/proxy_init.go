package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type ProxyInitializer struct{}

func (p *ProxyInitializer) Name() string {
	return "Proxy"
}

func (p *ProxyInitializer) Priority() int {
	return 500 // After most other built-ins
}

func (p *ProxyInitializer) InitTypes(ctx *TypeContext) error {
	handlerGeneric := proxyHandlerType()
	if err := ctx.DefineTypeAlias("ProxyHandler", handlerGeneric); err != nil {
		return err
	}

	// Create Proxy constructor type with both call signature and static methods
	proxyConstructor := types.NewObjectType()

	// new <T extends object>(target: T, handler: ProxyHandler<T>): T, as
	// lib.d.ts's ProxyConstructor (which cannot be called without new).
	target := &types.TypeParameter{Name: "T", Constraint: types.NonPrimitive}
	targetType := &types.TypeParameterType{Parameter: target}
	constructSignature := &types.Signature{
		TypeParameters: []*types.TypeParameter{target},
		ParameterNames: []string{"target", "handler"},
		ParameterTypes: []types.Type{targetType, types.NewInstantiatedType(handlerGeneric, []types.Type{targetType})},
		ReturnType:     targetType,
		OptionalParams: []bool{false, false},
	}
	proxyConstructor.Materialize()
	proxyConstructor.ConstructSignatures = []*types.Signature{constructSignature}

	// revocable<T extends object>(target: T, handler: ProxyHandler<T>): { proxy: T; revoke: () => void }
	revocableTarget := &types.TypeParameter{Name: "T", Constraint: types.NonPrimitive}
	revocableTargetType := &types.TypeParameterType{Parameter: revocableTarget}
	revocableReturnType := types.NewObjectType()
	revocableReturnType.WithProperty("proxy", revocableTargetType)
	revocableReturnType.WithProperty("revoke", types.NewSimpleFunction([]types.Type{}, types.Void))
	revocableType := types.NewObjectType()
	revocableType.Materialize()
	revocableType.CallSignatures = []*types.Signature{{
		TypeParameters: []*types.TypeParameter{revocableTarget},
		ParameterNames: []string{"target", "handler"},
		ParameterTypes: []types.Type{revocableTargetType, types.NewInstantiatedType(handlerGeneric, []types.Type{revocableTargetType})},
		ReturnType:     revocableReturnType,
		OptionalParams: []bool{false, false},
	}}
	proxyConstructor.WithProperty("revocable", revocableType)

	return ctx.DefineGlobal("Proxy", proxyConstructor)
}

// proxyHandlerType is lib.d.ts's ProxyHandler<T extends object>: every trap
// is an optional method of the target. Property descriptors are any, as in
// Object's typings.
func proxyHandlerType() *types.GenericType {
	t := &types.TypeParameter{Name: "T", Constraint: types.NonPrimitive}
	target := &types.TypeParameterType{Parameter: t}
	key := types.NewUnionType(types.String, types.Symbol)
	trap := func(names []string, params []types.Type, ret types.Type) *types.ObjectType {
		fn := types.NewObjectType()
		fn.Materialize()
		fn.CallSignatures = []*types.Signature{{
			ParameterNames: names,
			ParameterTypes: params,
			ReturnType:     ret,
			OptionalParams: make([]bool, len(params)),
		}}
		return fn
	}
	anyArray := &types.ArrayType{ElementType: types.Any}
	body := types.NewObjectType()
	body.WithOptionalProperty("apply", trap([]string{"target", "thisArg", "argArray"}, []types.Type{target, types.Any, anyArray}, types.Any))
	body.WithOptionalProperty("construct", trap([]string{"target", "argArray", "newTarget"}, []types.Type{target, anyArray, types.Any}, types.NonPrimitive))
	body.WithOptionalProperty("defineProperty", trap([]string{"target", "property", "attributes"}, []types.Type{target, key, types.Any}, types.Boolean))
	body.WithOptionalProperty("deleteProperty", trap([]string{"target", "p"}, []types.Type{target, key}, types.Boolean))
	body.WithOptionalProperty("get", trap([]string{"target", "p", "receiver"}, []types.Type{target, key, types.Any}, types.Any))
	body.WithOptionalProperty("getOwnPropertyDescriptor", trap([]string{"target", "p"}, []types.Type{target, key}, types.Any))
	body.WithOptionalProperty("getPrototypeOf", trap([]string{"target"}, []types.Type{target}, types.NewUnionType(types.NonPrimitive, types.Null)))
	body.WithOptionalProperty("has", trap([]string{"target", "p"}, []types.Type{target, key}, types.Boolean))
	body.WithOptionalProperty("isExtensible", trap([]string{"target"}, []types.Type{target}, types.Boolean))
	body.WithOptionalProperty("ownKeys", trap([]string{"target"}, []types.Type{target}, &types.ArrayType{ElementType: key}))
	body.WithOptionalProperty("preventExtensions", trap([]string{"target"}, []types.Type{target}, types.Boolean))
	body.WithOptionalProperty("set", trap([]string{"target", "p", "newValue", "receiver"}, []types.Type{target, key, types.Any, types.Any}, types.Boolean))
	body.WithOptionalProperty("setPrototypeOf", trap([]string{"target", "v"}, []types.Type{target, types.NewUnionType(types.NonPrimitive, types.Null)}, types.Boolean))
	return types.NewGenericType("ProxyHandler", []*types.TypeParameter{t}, body)
}

func (p *ProxyInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Proxy constructor
	proxyConstructor := vm.NewConstructorWithProps(2, false, "Proxy", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Proxy constructor requires target and handler arguments")
		}

		target := args[0]
		handler := args[1]

		// Validate target and handler
		// Target can be any object type (including arrays, maps, sets, etc.) or any callable type
		if !target.IsObject() && !target.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Proxy target must be an object or function")
		}

		if !handler.IsObject() {
			return vm.Undefined, vmInstance.NewTypeError("Proxy handler must be an object")
		}

		return vm.NewProxy(target, handler), nil
	})

	// Note: Proxy constructor deliberately has no usable .prototype property
	// Proxies inherit from their target's prototype, not from Proxy.prototype

	// Create Proxy.revocable static method
	revocableFn := vm.NewNativeFunction(2, false, "Proxy.revocable", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Proxy.revocable requires target and handler arguments")
		}

		target := args[0]
		handler := args[1]

		// Validate target and handler
		// Target can be any object type (including arrays, maps, sets, etc.) or any callable type
		if !target.IsObject() && !target.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Proxy target must be an object or function")
		}

		if !handler.IsObject() {
			return vm.Undefined, vmInstance.NewTypeError("Proxy handler must be an object")
		}

		// Create the proxy
		proxy := vm.NewProxy(target, handler)

		// Create the revoke function
		revokeFn := vm.NewNativeFunction(0, false, "revoke", func([]vm.Value) (vm.Value, error) {
			// Revoke the proxy
			proxy.AsProxy().Revoked = true
			return vm.Undefined, nil
		})

		// Create the result object
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		result.SetOwnNonEnumerable("proxy", proxy)
		result.SetOwnNonEnumerable("revoke", revokeFn)

		return vm.NewValueFromPlainObject(result), nil
	})

	// Add Proxy.revocable as a property on the constructor to match type system
	proxyConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("revocable", revocableFn)

	// Define Proxy constructor in global scope
	return ctx.DefineGlobal("Proxy", proxyConstructor)
}
