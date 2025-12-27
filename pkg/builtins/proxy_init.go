package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

type ProxyInitializer struct{}

func (p *ProxyInitializer) Name() string {
	return "Proxy"
}

func (p *ProxyInitializer) Priority() int {
	return 500 // After most other built-ins
}

func (p *ProxyInitializer) InitTypes(ctx *TypeContext) error {
	// Create Proxy constructor type with both call signature and static methods
	proxyConstructor := types.NewObjectType()

	// Add call signature (for new Proxy(...))
	callSignature := &types.Signature{
		ParameterTypes: []types.Type{types.Any, types.Any},
		ReturnType:     types.Any,
		OptionalParams: []bool{false, false},
	}
	proxyConstructor.CallSignatures = []*types.Signature{callSignature}

	// Add Proxy.revocable static method
	revocableReturnType := types.NewObjectType()
	revocableReturnType.WithProperty("proxy", types.Any)
	revocableReturnType.WithProperty("revoke", types.NewSimpleFunction([]types.Type{}, types.Void))

	revocableType := types.NewSimpleFunction([]types.Type{types.Any, types.Any}, revocableReturnType)
	proxyConstructor.WithProperty("revocable", revocableType)

	return ctx.DefineGlobal("Proxy", proxyConstructor)
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
		if !target.IsObject() && target.Type() != vm.TypeArray && target.Type() != vm.TypeFunction {
			return vm.Undefined, vmInstance.NewTypeError("Proxy target must be an object, array, or function")
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
		if !target.IsObject() && target.Type() != vm.TypeArray && target.Type() != vm.TypeFunction {
			return vm.Undefined, vmInstance.NewTypeError("Proxy target must be an object, array, or function")
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
