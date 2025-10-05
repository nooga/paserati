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
		)).
		WithProperty("all", types.NewSimpleFunction(
			[]types.Type{types.Any}, // iterable
			types.Any,               // Promise<any[]>
		)).
		WithProperty("race", types.NewSimpleFunction(
			[]types.Type{types.Any}, // iterable
			types.Any,               // Promise<any>
		)).
		WithProperty("allSettled", types.NewSimpleFunction(
			[]types.Type{types.Any}, // iterable
			types.Any,               // Promise<PromiseSettledResult[]>
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

	// Promise.all(iterable)
	props.SetOwn("all", vm.NewNativeFunction(1, false, "all", func(args []vm.Value) (vm.Value, error) {
		iterable := vm.Undefined
		if len(args) > 0 {
			iterable = args[0]
		}

		// Convert iterable to array
		arr, err := vmInstance.IterableToArray(iterable)
		if err != nil {
			return vm.Undefined, fmt.Errorf("TypeError: Promise.all requires an iterable")
		}

		arrayObj := arr.AsArray()
		if arrayObj == nil {
			return vm.Undefined, fmt.Errorf("TypeError: Promise.all requires an iterable")
		}

		length := arrayObj.Length()
		if length == 0 {
			// Empty array resolves immediately to empty array
			return vmInstance.NewResolvedPromise(arr), nil
		}

		// Create a new promise that resolves when all promises resolve
		executor := vm.NewNativeFunction(2, false, "executor", func(execArgs []vm.Value) (vm.Value, error) {
			resolve := execArgs[0]
			reject := execArgs[1]

			// Track results and completion count
			results := make([]vm.Value, length)
			remaining := length

			// Attach handlers to each promise
			for i := 0; i < length; i++ {
				idx := i                        // Capture index for closure
				promiseOrValue := arrayObj.Get(i)

				// Convert non-promises to resolved promises
				var promise vm.Value
				if promiseOrValue.Type() == vm.TypePromise {
					promise = promiseOrValue
				} else {
					promise = vmInstance.NewResolvedPromise(promiseOrValue)
				}

				// Attach fulfillment handler
				onFulfilled := vm.NewNativeFunction(1, false, "onFulfilled", func(valueArgs []vm.Value) (vm.Value, error) {
					value := vm.Undefined
					if len(valueArgs) > 0 {
						value = valueArgs[0]
					}

					results[idx] = value
					remaining--

					if remaining == 0 {
						// All promises resolved - create result array
						resultArray := vmInstance.NewArrayFromSlice(results)
						vmInstance.Call(resolve, vm.Undefined, []vm.Value{resultArray})
					}

					return vm.Undefined, nil
				})

				// Attach rejection handler
				onRejected := vm.NewNativeFunction(1, false, "onRejected", func(reasonArgs []vm.Value) (vm.Value, error) {
					reason := vm.Undefined
					if len(reasonArgs) > 0 {
						reason = reasonArgs[0]
					}

					// Reject the entire Promise.all
					vmInstance.Call(reject, vm.Undefined, []vm.Value{reason})
					return vm.Undefined, nil
				})

				// Attach handlers
				vmInstance.PromiseThen(promise, onFulfilled, onRejected)
			}

			return vm.Undefined, nil
		})

		return vmInstance.NewPromiseFromExecutor(executor)
	}))

	// Promise.race(iterable)
	props.SetOwn("race", vm.NewNativeFunction(1, false, "race", func(args []vm.Value) (vm.Value, error) {
		iterable := vm.Undefined
		if len(args) > 0 {
			iterable = args[0]
		}

		// Convert iterable to array
		arr, err := vmInstance.IterableToArray(iterable)
		if err != nil {
			return vm.Undefined, fmt.Errorf("TypeError: Promise.race requires an iterable")
		}

		arrayObj := arr.AsArray()
		if arrayObj == nil {
			return vm.Undefined, fmt.Errorf("TypeError: Promise.race requires an iterable")
		}

		length := arrayObj.Length()

		// Create a new promise that settles when the first promise settles
		executor := vm.NewNativeFunction(2, false, "executor", func(execArgs []vm.Value) (vm.Value, error) {
			resolve := execArgs[0]
			reject := execArgs[1]

			if length == 0 {
				// Empty array - promise never settles (per ECMAScript spec)
				return vm.Undefined, nil
			}

			// Attach handlers to each promise
			for i := 0; i < length; i++ {
				promiseOrValue := arrayObj.Get(i)
				// Convert non-promises to resolved promises
				var promise vm.Value
				if promiseOrValue.Type() == vm.TypePromise {
					promise = promiseOrValue
				} else {
					promise = vmInstance.NewResolvedPromise(promiseOrValue)
				}

				// Attach fulfillment handler
				onFulfilled := vm.NewNativeFunction(1, false, "onFulfilled", func(valueArgs []vm.Value) (vm.Value, error) {
					value := vm.Undefined
					if len(valueArgs) > 0 {
						value = valueArgs[0]
					}

					// Resolve with the first settled value
					vmInstance.Call(resolve, vm.Undefined, []vm.Value{value})
					return vm.Undefined, nil
				})

				// Attach rejection handler
				onRejected := vm.NewNativeFunction(1, false, "onRejected", func(reasonArgs []vm.Value) (vm.Value, error) {
					reason := vm.Undefined
					if len(reasonArgs) > 0 {
						reason = reasonArgs[0]
					}

					// Reject with the first rejection reason
					vmInstance.Call(reject, vm.Undefined, []vm.Value{reason})
					return vm.Undefined, nil
				})

				// Attach handlers
				vmInstance.PromiseThen(promise, onFulfilled, onRejected)
			}

			return vm.Undefined, nil
		})

		return vmInstance.NewPromiseFromExecutor(executor)
	}))

	// Promise.allSettled(iterable)
	props.SetOwn("allSettled", vm.NewNativeFunction(1, false, "allSettled", func(args []vm.Value) (vm.Value, error) {
		iterable := vm.Undefined
		if len(args) > 0 {
			iterable = args[0]
		}

		// Convert iterable to array
		arr, err := vmInstance.IterableToArray(iterable)
		if err != nil {
			return vm.Undefined, fmt.Errorf("TypeError: Promise.allSettled requires an iterable")
		}

		arrayObj := arr.AsArray()
		if arrayObj == nil {
			return vm.Undefined, fmt.Errorf("TypeError: Promise.allSettled requires an iterable")
		}

		length := arrayObj.Length()
		if length == 0 {
			// Empty array resolves immediately to empty array
			return vmInstance.NewResolvedPromise(arr), nil
		}

		// Create a new promise that resolves when all promises settle
		executor := vm.NewNativeFunction(2, false, "executor", func(execArgs []vm.Value) (vm.Value, error) {
			resolve := execArgs[0]

			// Track results and completion count
			results := make([]vm.Value, length)
			remaining := length

			// Attach handlers to each promise
			for i := 0; i < length; i++ {
				idx := i                        // Capture index for closure
				promiseOrValue := arrayObj.Get(i)

				// Convert non-promises to resolved promises
				var promise vm.Value
				if promiseOrValue.Type() == vm.TypePromise {
					promise = promiseOrValue
				} else {
					promise = vmInstance.NewResolvedPromise(promiseOrValue)
				}

				// Attach fulfillment handler
				onFulfilled := vm.NewNativeFunction(1, false, "onFulfilled", func(valueArgs []vm.Value) (vm.Value, error) {
					value := vm.Undefined
					if len(valueArgs) > 0 {
						value = valueArgs[0]
					}

					// Create { status: "fulfilled", value: ... } object
					resultObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					resultObj.SetOwn("status", vm.NewString("fulfilled"))
					resultObj.SetOwn("value", value)

					results[idx] = vm.NewValueFromPlainObject(resultObj)
					remaining--

					if remaining == 0 {
						// All promises settled - create result array
						resultArray := vmInstance.NewArrayFromSlice(results)
						vmInstance.Call(resolve, vm.Undefined, []vm.Value{resultArray})
					}

					return vm.Undefined, nil
				})

				// Attach rejection handler
				onRejected := vm.NewNativeFunction(1, false, "onRejected", func(reasonArgs []vm.Value) (vm.Value, error) {
					reason := vm.Undefined
					if len(reasonArgs) > 0 {
						reason = reasonArgs[0]
					}

					// Create { status: "rejected", reason: ... } object
					resultObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					resultObj.SetOwn("status", vm.NewString("rejected"))
					resultObj.SetOwn("reason", reason)

					results[idx] = vm.NewValueFromPlainObject(resultObj)
					remaining--

					if remaining == 0 {
						// All promises settled - create result array
						resultArray := vmInstance.NewArrayFromSlice(results)
						vmInstance.Call(resolve, vm.Undefined, []vm.Value{resultArray})
					}

					return vm.Undefined, nil
				})

				// Attach handlers
				vmInstance.PromiseThen(promise, onFulfilled, onRejected)
			}

			return vm.Undefined, nil
		})

		return vmInstance.NewPromiseFromExecutor(executor)
	}))

	// Set constructor property on prototype
	promiseProto.SetOwn("constructor", promiseCtor)

	// Register Promise constructor as global
	return ctx.DefineGlobal("Promise", promiseCtor)
}
