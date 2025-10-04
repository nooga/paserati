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
	// Create Proxy constructor type
	proxyType := types.NewSimpleFunction([]types.Type{types.Any, types.Any}, types.Any)

	err := ctx.DefineGlobal("Proxy", proxyType)
	if err != nil {
		return err
	}

	// Also define Proxy.revocable
	revocableReturnType := types.NewObjectType()
	revocableReturnType.WithProperty("proxy", types.Any)
	revocableReturnType.WithProperty("revoke", types.NewSimpleFunction([]types.Type{}, types.Void))

	revocableType := types.NewSimpleFunction([]types.Type{types.Any, types.Any}, revocableReturnType)

	return ctx.DefineGlobal("Proxy.revocable", revocableType)
}

func (p *ProxyInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Proxy constructor
	proxyConstructor := vm.NewNativeFunctionWithProps(2, false, "Proxy", func(args []vm.Value) (vm.Value, error) {
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

	// Add prototype property
	proxyConstructor.AsNativeFunctionWithProps().Properties.SetOwn("prototype", vmInstance.ObjectPrototype)
	if v, ok := proxyConstructor.AsNativeFunctionWithProps().Properties.GetOwn("prototype"); ok {
		w, e, c := false, false, false
		proxyConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("prototype", v, &w, &e, &c)
	}

	// Add Proxy.revocable
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
		result.SetOwn("proxy", proxy)
		result.SetOwn("revoke", revokeFn)

		return vm.NewValueFromPlainObject(result), nil
	})

	// Define Proxy constructor in global scope
	err := ctx.DefineGlobal("Proxy", proxyConstructor)
	if err != nil {
		return err
	}

	return ctx.DefineGlobal("Proxy.revocable", revocableFn)
}
