package builtins

import (
	"fmt"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

type PromiseInitializer struct{}

func (p *PromiseInitializer) Name() string {
	return "Promise"
}

func (p *PromiseInitializer) Priority() int {
	return PriorityGenerator + 1 // After generators, before other builtins
}

func (p *PromiseInitializer) InitTypes(ctx *TypeContext) error {
	// Create Promise.prototype type (simplified for now)
	promiseProtoType := types.NewObjectType().
		WithProperty("then", types.NewSimpleFunction(
			[]types.Type{types.Any, types.Any},
			types.Any,
		)).
		WithProperty("catch", types.NewSimpleFunction(
			[]types.Type{types.Any},
			types.Any,
		)).
		WithProperty("finally", types.NewSimpleFunction(
			[]types.Type{types.Any},
			types.Any,
		))

	// Create Promise constructor type
	promiseCtorType := types.NewObjectType().
		WithProperty("prototype", promiseProtoType).
		WithProperty("resolve", types.NewSimpleFunction(
			[]types.Type{types.Any},
			types.Any,
		)).
		WithProperty("reject", types.NewSimpleFunction(
			[]types.Type{types.Any},
			types.Any,
		))

	// Add call signature for Promise constructor
	executorType := types.NewSimpleFunction(
		[]types.Type{types.Any, types.Any}, // resolve, reject
		types.Void,
	)
	promiseCtorType = promiseCtorType.WithSimpleCallSignature([]types.Type{executorType}, promiseProtoType)

	return ctx.DefineGlobal("Promise", promiseCtorType)
}

func (p *PromiseInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create Promise.prototype inheriting from Object.prototype
	promiseProto := vm.NewObject(objectProto).AsPlainObject()

	// Promise.prototype.then(onFulfilled, onRejected)
	promiseProto.SetOwn("then", vm.NewNativeFunction(2, false, "then", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		onFulfilled := vm.Undefined
		onRejected := vm.Undefined

		if len(args) > 0 {
			onFulfilled = args[0]
		}
		if len(args) > 1 {
			onRejected = args[1]
		}

		return vmInstance.PromiseThen(thisVal, onFulfilled, onRejected)
	}))

	// Promise.prototype.catch(onRejected)
	promiseProto.SetOwn("catch", vm.NewNativeFunction(1, false, "catch", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		onRejected := vm.Undefined
		if len(args) > 0 {
			onRejected = args[0]
		}

		// catch(onRejected) is equivalent to then(undefined, onRejected)
		return vmInstance.PromiseThen(thisVal, vm.Undefined, onRejected)
	}))

	// Promise.prototype.finally(onFinally)
	promiseProto.SetOwn("finally", vm.NewNativeFunction(1, false, "finally", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		onFinally := vm.Undefined
		if len(args) > 0 {
			onFinally = args[0]
		}

		// finally wraps both fulfill and reject handlers
		wrapper := vm.NewNativeFunction(1, false, "finallyWrapper", func(wrapperArgs []vm.Value) (vm.Value, error) {
			if onFinally.IsCallable() {
				vmInstance.Call(onFinally, vm.Undefined, []vm.Value{})
			}
			// Pass through the original value
			if len(wrapperArgs) > 0 {
				return wrapperArgs[0], nil
			}
			return vm.Undefined, nil
		})

		return vmInstance.PromiseThen(thisVal, wrapper, wrapper)
	}))

	// Store Promise.prototype on VM
	vmInstance.PromisePrototype = vm.NewValueFromPlainObject(promiseProto)

	// Create Promise constructor
	promiseCtor := vm.NewNativeFunctionWithProps(1, true, "Promise", func(args []vm.Value) (vm.Value, error) {
		executor := vm.Undefined
		if len(args) > 0 {
			executor = args[0]
		}

		if !executor.IsCallable() {
			return vm.Undefined, fmt.Errorf("TypeError: Promise resolver %s is not a function", executor.TypeName())
		}

		return vmInstance.NewPromiseFromExecutor(executor)
	})

	// Add static methods to Promise constructor
	props := promiseCtor.AsNativeFunctionWithProps().Properties

	// Promise.prototype
	props.SetOwn("prototype", vmInstance.PromisePrototype)

	// Promise.resolve(value)
	props.SetOwn("resolve", vm.NewNativeFunction(1, false, "resolve", func(args []vm.Value) (vm.Value, error) {
		value := vm.Undefined
		if len(args) > 0 {
			value = args[0]
		}

		// If value is already a promise, return it
		if value.Type() == vm.TypePromise {
			return value, nil
		}

		return vmInstance.NewResolvedPromise(value), nil
	}))

	// Promise.reject(reason)
	props.SetOwn("reject", vm.NewNativeFunction(1, false, "reject", func(args []vm.Value) (vm.Value, error) {
		reason := vm.Undefined
		if len(args) > 0 {
			reason = args[0]
		}

		return vmInstance.NewRejectedPromise(reason), nil
	}))

	// Set constructor property on prototype
	promiseProto.SetOwn("constructor", promiseCtor)

	// Register Promise constructor as global
	return ctx.DefineGlobal("Promise", promiseCtor)
}
