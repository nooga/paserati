package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
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
		)).
		WithProperty("any", types.NewSimpleFunction(
			[]types.Type{types.Any}, // iterable
			types.Any,               // Promise<any>
		)).
		WithProperty("try", types.NewVariadicFunction(
			[]types.Type{types.Any},                  // callbackfn
			types.Any,                                // Promise<any>
			&types.ArrayType{ElementType: types.Any}, // ...args
		))

	// Add call signature for Promise constructor
	executorType := types.NewSimpleFunction(
		[]types.Type{types.Any, types.Any}, // resolve, reject
		types.Void,
	)
	promiseCtorType = promiseCtorType.WithSimpleCallSignature([]types.Type{executorType}, promiseProtoType)

	return ctx.DefineGlobal("Promise", promiseCtorType)
}

// promiseCapabilityGuard returns a per-executor closure implementing the
// bounds-safe argument extraction and "already called" TypeError guard from
// the spec's GetCapabilitiesExecutor Functions (called by NewPromiseCapability):
// resolve/reject default to undefined when the caller invokes the executor
// with fewer than 2 arguments (a hand-rolled "constructor" passed to
// Promise.all/race/etc. via .call() is free to do this), and a second call
// throws once either slot already holds a non-undefined value. Call once per
// executor closure - the guard must be called at the top of the native body.
func promiseCapabilityGuard(vmInstance *vm.VM) func(execArgs []vm.Value) (vm.Value, vm.Value, error) {
	capResolve, capReject := vm.Undefined, vm.Undefined
	return func(execArgs []vm.Value) (vm.Value, vm.Value, error) {
		resolve := vm.Undefined
		if len(execArgs) > 0 {
			resolve = execArgs[0]
		}
		reject := vm.Undefined
		if len(execArgs) > 1 {
			reject = execArgs[1]
		}
		if capResolve.Type() != vm.TypeUndefined {
			return vm.Undefined, vm.Undefined, vmInstance.NewTypeError("Promise capability's resolve function was already set")
		}
		if capReject.Type() != vm.TypeUndefined {
			return vm.Undefined, vm.Undefined, vmInstance.NewTypeError("Promise capability's reject function was already set")
		}
		capResolve, capReject = resolve, reject
		return capResolve, capReject, nil
	}
}

func (p *PromiseInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create Promise.prototype inheriting from Object.prototype
	promiseProto := vm.NewObject(objectProto).AsPlainObject()

	// Promise.prototype.then(onFulfilled, onRejected)
	promiseProto.SetOwnNonEnumerable("then", vm.NewNativeFunction(2, false, "then", func(args []vm.Value) (vm.Value, error) {
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
	promiseProto.SetOwnNonEnumerable("catch", vm.NewNativeFunction(1, false, "catch", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		onRejected := vm.Undefined
		if len(args) > 0 {
			onRejected = args[0]
		}

		// catch(onRejected) is equivalent to then(undefined, onRejected)
		return vmInstance.PromiseThen(thisVal, vm.Undefined, onRejected)
	}))

	// Promise.prototype.finally(onFinally)
	promiseProto.SetOwnNonEnumerable("finally", vm.NewNativeFunction(1, false, "finally", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		onFinally := vm.Undefined
		if len(args) > 0 {
			onFinally = args[0]
		}

		// finally wraps both fulfill and reject handlers
		wrapper := vm.NewNativeFunction(1, false, "finallyWrapper", func(wrapperArgs []vm.Value) (vm.Value, error) {
			if onFinally.IsCallable() {
				_, _ = vmInstance.Call(onFinally, vm.Undefined, []vm.Value{})
			}
			// Pass through the original value
			if len(wrapperArgs) > 0 {
				return wrapperArgs[0], nil
			}
			return vm.Undefined, nil
		})

		return vmInstance.PromiseThen(thisVal, wrapper, wrapper)
	}))

	// Add Promise.prototype[@@toStringTag] = "Promise" (writable: false, enumerable: false, configurable: true)
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		wFalse, eFalse, cTrue := false, false, true
		promiseProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Promise"),
			&wFalse, &eFalse, &cTrue,
		)
	}

	// Store Promise.prototype on VM
	vmInstance.PromisePrototype = vm.NewValueFromPlainObject(promiseProto)

	// Create Promise constructor
	promiseCtor := vm.NewConstructorWithProps(1, true, "Promise", func(args []vm.Value) (vm.Value, error) {
		executor := vm.Undefined
		if len(args) > 0 {
			executor = args[0]
		}

		if !executor.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Promise resolver " + executor.TypeName() + " is not a function")
		}

		// Per ECMAScript 25.6.3.1 step 3: OrdinaryCreateFromConstructor(NewTarget, "%Promise.prototype%")
		if newTarget := vmInstance.GetNewTarget(); !newTarget.IsUndefined() {
			_, gpfcErr := vmInstance.GetPrototypeFromConstructor(newTarget, "%PromisePrototype%")
			if gpfcErr != nil {
				return vm.Undefined, gpfcErr
			}
		}

		return vmInstance.NewPromiseFromExecutor(executor)
	})

	// Add static methods to Promise constructor
	props := promiseCtor.AsNativeFunctionWithProps().Properties

	// Promise.prototype
	props.DefineFixedProperty("prototype", vmInstance.PromisePrototype)

	// Promise.resolve(value)
	props.SetOwnNonEnumerable("resolve", vm.NewNativeFunction(1, false, "resolve", func(args []vm.Value) (vm.Value, error) {
		// Step 1-2: Let C be the this value. If Type(C) is not Object, throw TypeError.
		thisVal := vmInstance.GetThis()
		if !thisVal.IsObject() && !thisVal.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Promise.resolve called on non-object")
		}

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
	props.SetOwnNonEnumerable("reject", vm.NewNativeFunction(1, false, "reject", func(args []vm.Value) (vm.Value, error) {
		// Step 1-2: Let C be the this value. If Type(C) is not Object, throw TypeError.
		thisVal := vmInstance.GetThis()
		if !thisVal.IsObject() && !thisVal.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Promise.reject called on non-object")
		}

		reason := vm.Undefined
		if len(args) > 0 {
			reason = args[0]
		}

		return vmInstance.NewRejectedPromise(reason), nil
	}))

	// Promise[Symbol.species] - should be a getter that returns 'this'
	// For now, just set it to Promise itself (simpler, covers most cases)
	props.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolSpecies), promiseCtor, nil, nil, nil)

	// Helper: Get the species constructor from 'this' or fall back to Promise
	getSpeciesConstructor := func(thisVal vm.Value) vm.Value {
		// Try to get this[Symbol.species]
		if thisVal.IsObject() || thisVal.Type() == vm.TypeNativeFunctionWithProps {
			var speciesVal vm.Value

			// Try to get Symbol.species property
			if thisVal.Type() == vm.TypeNativeFunctionWithProps {
				nfp := thisVal.AsNativeFunctionWithProps()
				if species, exists := nfp.Properties.GetOwnByKey(vm.NewSymbolKey(SymbolSpecies)); exists {
					speciesVal = species
				}
			}

			// If species is defined and not null/undefined, use it
			if speciesVal.Type() != vm.TypeUndefined && speciesVal.Type() != vm.TypeNull {
				return speciesVal
			}
		}

		// Fall back to 'this' value (the constructor itself)
		return thisVal
	}

	// getPromiseResolve implements GetPromiseResolve(C) per ECMAScript spec.
	// Returns C.resolve if it's callable, otherwise returns an error.
	getPromiseResolve := func(constructor vm.Value) (vm.Value, error) {
		resolve, err := vmInstance.GetProperty(constructor, "resolve")
		if err != nil {
			return vm.Undefined, err
		}
		if !resolve.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Promise resolve is not a function")
		}
		return resolve, nil
	}

	// invokeThen calls .then(onFulfilled, onRejected) on any value.
	// For native Promises, uses PromiseThen directly. For other objects, calls .then() method.
	invokeThen := func(obj, onFulfilled, onRejected vm.Value) (vm.Value, error) {
		if obj.Type() == vm.TypePromise {
			return vmInstance.PromiseThen(obj, onFulfilled, onRejected)
		}
		thenMethod, err := vmInstance.GetProperty(obj, "then")
		if err != nil {
			return vm.Undefined, err
		}
		if thenMethod.IsCallable() {
			return vmInstance.CallArgs2(thenMethod, obj, onFulfilled, onRejected)
		}
		// If no .then() method, wrap in a resolved promise and chain
		return vmInstance.PromiseThen(vmInstance.NewResolvedPromise(obj), onFulfilled, onRejected)
	}

	// Promise.all(iterable)
	props.SetOwnNonEnumerable("all", vm.NewNativeFunction(1, false, "all", func(args []vm.Value) (vm.Value, error) {
		iterable := vm.Undefined
		if len(args) > 0 {
			iterable = args[0]
		}

		// Step 1-2: Let C be the this value. If Type(C) is not Object, throw TypeError.
		thisVal := vmInstance.GetThis()
		if !thisVal.IsObject() && !thisVal.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Promise.all called on non-object")
		}
		constructor := getSpeciesConstructor(thisVal)

		// Convert iterable to array (before promise creation per spec)
		arr, err := vmInstance.IterableToArray(iterable)
		if err != nil {
			return vm.Undefined, vmInstance.NewTypeError("Promise.all requires an iterable")
		}

		arrayObj := arr.AsArray()
		if arrayObj == nil {
			return vm.Undefined, vmInstance.NewTypeError("Promise.all requires an iterable")
		}

		length := arrayObj.Length()

		// Create the result promise via executor
		// Per spec: NewPromiseCapability(C) first, then GetPromiseResolve(C),
		// then IfAbruptRejectPromise if it fails
		capabilityGuard := promiseCapabilityGuard(vmInstance)
		executor := vm.NewNativeFunction(2, false, "executor", func(execArgs []vm.Value) (vm.Value, error) {
			resolve, reject, guardErr := capabilityGuard(execArgs)
			if guardErr != nil {
				return vm.Undefined, guardErr
			}

			if length == 0 {
				_, _ = vmInstance.Call(resolve, vm.Undefined, []vm.Value{arr})
				return vm.Undefined, nil
			}

			// GetPromiseResolve(C) - per spec, if this fails, reject the promise (IfAbruptRejectPromise)
			promiseResolve, resolveErr := getPromiseResolve(constructor)
			if resolveErr != nil {
				errVal := vm.NewString(resolveErr.Error())
				if ee, ok := resolveErr.(vm.ExceptionError); ok {
					errVal = ee.GetExceptionValue()
				}
				vmInstance.ClearUnwindingState()
				_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{errVal})
				return vm.Undefined, nil
			}

			// Track results and completion count
			results := make([]vm.Value, length)
			remaining := length

			// Attach handlers to each promise
			for i := 0; i < length; i++ {
				idx := i // Capture index for closure
				promiseOrValue := arrayObj.Get(i)

				// Call C.resolve(promiseOrValue) per spec
				nextPromise, callErr := vmInstance.Call(promiseResolve, constructor, []vm.Value{promiseOrValue})
				if callErr != nil {
					errVal := vm.NewString(callErr.Error())
					if ee, ok := callErr.(vm.ExceptionError); ok {
						errVal = ee.GetExceptionValue()
					}
					vmInstance.ClearUnwindingState()
					_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{errVal})
					return vm.Undefined, nil
				}

				// Attach fulfillment handler (not a constructor per spec)
				// Per ECMAScript 25.4.4.1.2 (Promise.all Resolve Element
				// Functions) step 1-3: each element function has its own
				// [[AlreadyCalled]] flag and must no-op on a second call. This
				// is separate from (and needed in addition to) a real Promise
				// capability's own idempotent resolve/reject: nextPromise here
				// can be an arbitrary thenable that calls onFulfilled more
				// than once, and without this guard doing so double-decrements
				// `remaining` and can invoke `resolve` (or overwrite settled
				// results) more times than the spec allows.
				alreadyCalled := false
				onFulfilled := vm.NewNativeFunction(1, false, "onFulfilled", func(valueArgs []vm.Value) (vm.Value, error) {
					if alreadyCalled {
						return vm.Undefined, nil
					}
					alreadyCalled = true

					value := vm.Undefined
					if len(valueArgs) > 0 {
						value = valueArgs[0]
					}

					results[idx] = value
					remaining--

					if remaining == 0 {
						// All promises resolved - create result array
						resultArray := vmInstance.NewArrayFromSlice(results)
						_, _ = vmInstance.Call(resolve, vm.Undefined, []vm.Value{resultArray})
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
					_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{reason})
					return vm.Undefined, nil
				})

				// Attach handlers via .then()
				if _, thenErr := invokeThen(nextPromise, onFulfilled, onRejected); thenErr != nil {
					// Calling .then() itself threw (e.g. a thenable resolving to
					// itself). Per spec this should reject the result promise
					// (IfAbruptRejectPromise), not vanish - vm.Call leaves
					// vm.unwinding set on error for legitimate re-throw callers;
					// we're absorbing it into a rejection instead, so it must be
					// cleared or it leaks into whatever bytecode called this
					// static method.
					errVal := vm.NewString(thenErr.Error())
					if ee, ok := thenErr.(vm.ExceptionError); ok {
						errVal = ee.GetExceptionValue()
					}
					vmInstance.ClearUnwindingState()
					_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{errVal})
					return vm.Undefined, nil
				}
			}

			return vm.Undefined, nil
		})

		// Use the species constructor to create the result promise
		if constructor.IsCallable() {
			return vmInstance.Call(constructor, vm.Undefined, []vm.Value{executor})
		}
		return vmInstance.NewPromiseFromExecutor(executor)
	}))

	// Promise.race(iterable)
	props.SetOwnNonEnumerable("race", vm.NewNativeFunction(1, false, "race", func(args []vm.Value) (vm.Value, error) {
		iterable := vm.Undefined
		if len(args) > 0 {
			iterable = args[0]
		}

		// Step 1-2: Let C be the this value. If Type(C) is not Object, throw TypeError.
		thisVal := vmInstance.GetThis()
		if !thisVal.IsObject() && !thisVal.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Promise.race called on non-object")
		}
		constructor := getSpeciesConstructor(thisVal)

		// Convert iterable to array
		arr, err := vmInstance.IterableToArray(iterable)
		if err != nil {
			return vm.Undefined, vmInstance.NewTypeError("Promise.race requires an iterable")
		}

		arrayObj := arr.AsArray()
		if arrayObj == nil {
			return vm.Undefined, vmInstance.NewTypeError("Promise.race requires an iterable")
		}

		length := arrayObj.Length()

		// Create a new promise that settles when the first promise settles
		capabilityGuard := promiseCapabilityGuard(vmInstance)
		executor := vm.NewNativeFunction(2, false, "executor", func(execArgs []vm.Value) (vm.Value, error) {
			resolve, reject, guardErr := capabilityGuard(execArgs)
			if guardErr != nil {
				return vm.Undefined, guardErr
			}

			if length == 0 {
				// Empty array - promise never settles (per ECMAScript spec)
				return vm.Undefined, nil
			}

			// GetPromiseResolve(C) - per spec, if this fails, reject the promise
			promiseResolve, resolveErr := getPromiseResolve(constructor)
			if resolveErr != nil {
				errVal := vm.NewString(resolveErr.Error())
				if ee, ok := resolveErr.(vm.ExceptionError); ok {
					errVal = ee.GetExceptionValue()
				}
				vmInstance.ClearUnwindingState()
				_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{errVal})
				return vm.Undefined, nil
			}

			// Attach handlers to each promise
			for i := 0; i < length; i++ {
				promiseOrValue := arrayObj.Get(i)

				// Call C.resolve(promiseOrValue) per spec
				nextPromise, callErr := vmInstance.Call(promiseResolve, constructor, []vm.Value{promiseOrValue})
				if callErr != nil {
					errVal := vm.NewString(callErr.Error())
					if ee, ok := callErr.(vm.ExceptionError); ok {
						errVal = ee.GetExceptionValue()
					}
					vmInstance.ClearUnwindingState()
					_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{errVal})
					return vm.Undefined, nil
				}

				// Attach fulfillment handler
				onFulfilled := vm.NewNativeFunction(1, false, "onFulfilled", func(valueArgs []vm.Value) (vm.Value, error) {
					value := vm.Undefined
					if len(valueArgs) > 0 {
						value = valueArgs[0]
					}

					// Resolve with the first settled value
					_, _ = vmInstance.Call(resolve, vm.Undefined, []vm.Value{value})
					return vm.Undefined, nil
				})

				// Attach rejection handler
				onRejected := vm.NewNativeFunction(1, false, "onRejected", func(reasonArgs []vm.Value) (vm.Value, error) {
					reason := vm.Undefined
					if len(reasonArgs) > 0 {
						reason = reasonArgs[0]
					}

					// Reject with the first rejection reason
					_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{reason})
					return vm.Undefined, nil
				})

				// Attach handlers via .then()
				if _, thenErr := invokeThen(nextPromise, onFulfilled, onRejected); thenErr != nil {
					// Calling .then() itself threw (e.g. a thenable resolving to
					// itself). Per spec this should reject the result promise
					// (IfAbruptRejectPromise), not vanish - vm.Call leaves
					// vm.unwinding set on error for legitimate re-throw callers;
					// we're absorbing it into a rejection instead, so it must be
					// cleared or it leaks into whatever bytecode called this
					// static method.
					errVal := vm.NewString(thenErr.Error())
					if ee, ok := thenErr.(vm.ExceptionError); ok {
						errVal = ee.GetExceptionValue()
					}
					vmInstance.ClearUnwindingState()
					_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{errVal})
					return vm.Undefined, nil
				}
			}

			return vm.Undefined, nil
		})

		// Use the species constructor to create the result promise
		if constructor.IsCallable() {
			return vmInstance.Call(constructor, vm.Undefined, []vm.Value{executor})
		}
		return vmInstance.NewPromiseFromExecutor(executor)
	}))

	// Promise.any(iterable)
	props.SetOwnNonEnumerable("any", vm.NewNativeFunction(1, false, "any", func(args []vm.Value) (vm.Value, error) {
		iterable := vm.Undefined
		if len(args) > 0 {
			iterable = args[0]
		}

		// Step 1-2: Let C be the this value. If Type(C) is not Object, throw TypeError.
		thisVal := vmInstance.GetThis()
		if !thisVal.IsObject() && !thisVal.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Promise.any called on non-object")
		}
		constructor := getSpeciesConstructor(thisVal)

		// Convert iterable to array
		arr, err := vmInstance.IterableToArray(iterable)
		if err != nil {
			return vm.Undefined, vmInstance.NewTypeError("Promise.any requires an iterable")
		}

		arrayObj := arr.AsArray()
		if arrayObj == nil {
			return vm.Undefined, vmInstance.NewTypeError("Promise.any requires an iterable")
		}

		length := arrayObj.Length()
		if length == 0 {
			// Empty array - reject immediately with AggregateError
			// TODO: Implement proper AggregateError
			errorMsg := vm.NewString("AggregateError: All promises were rejected")
			return vmInstance.NewRejectedPromise(errorMsg), nil
		}

		// Create a new promise that resolves with the first fulfilled promise
		capabilityGuard := promiseCapabilityGuard(vmInstance)
		executor := vm.NewNativeFunction(2, false, "executor", func(execArgs []vm.Value) (vm.Value, error) {
			resolve, reject, guardErr := capabilityGuard(execArgs)
			if guardErr != nil {
				return vm.Undefined, guardErr
			}

			// GetPromiseResolve(C) - per spec, if this fails, reject the promise
			promiseResolve, resolveErr := getPromiseResolve(constructor)
			if resolveErr != nil {
				errVal := vm.NewString(resolveErr.Error())
				if ee, ok := resolveErr.(vm.ExceptionError); ok {
					errVal = ee.GetExceptionValue()
				}
				vmInstance.ClearUnwindingState()
				_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{errVal})
				return vm.Undefined, nil
			}

			// Track rejections and completion count
			errors := make([]vm.Value, length)
			remaining := length

			// Attach handlers to each promise
			for i := 0; i < length; i++ {
				idx := i // Capture index for closure
				promiseOrValue := arrayObj.Get(i)

				// Call C.resolve(promiseOrValue) per spec
				nextPromise, callErr := vmInstance.Call(promiseResolve, constructor, []vm.Value{promiseOrValue})
				if callErr != nil {
					errVal := vm.NewString(callErr.Error())
					if ee, ok := callErr.(vm.ExceptionError); ok {
						errVal = ee.GetExceptionValue()
					}
					vmInstance.ClearUnwindingState()
					_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{errVal})
					return vm.Undefined, nil
				}

				// Attach fulfillment handler
				onFulfilled := vm.NewNativeFunction(1, false, "onFulfilled", func(valueArgs []vm.Value) (vm.Value, error) {
					value := vm.Undefined
					if len(valueArgs) > 0 {
						value = valueArgs[0]
					}

					// Resolve with the first fulfilled value
					_, _ = vmInstance.Call(resolve, vm.Undefined, []vm.Value{value})
					return vm.Undefined, nil
				})

				// Attach rejection handler
				// See the matching comment on Promise.all's onFulfilled: nextPromise
				// can be an arbitrary thenable calling onRejected more than once,
				// and without a per-element AlreadyCalled guard that would
				// double-decrement `remaining` and overwrite/duplicate errors[idx].
				alreadyCalled := false
				onRejected := vm.NewNativeFunction(1, false, "onRejected", func(reasonArgs []vm.Value) (vm.Value, error) {
					if alreadyCalled {
						return vm.Undefined, nil
					}
					alreadyCalled = true

					reason := vm.Undefined
					if len(reasonArgs) > 0 {
						reason = reasonArgs[0]
					}

					// Store the error
					errors[idx] = reason
					remaining--

					// If all promises rejected, reject with AggregateError
					if remaining == 0 {
						// TODO: Create proper AggregateError with errors array
						// For now, just create a simple error message
						errorMsg := vm.NewString("AggregateError: All promises were rejected")
						_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{errorMsg})
					}

					return vm.Undefined, nil
				})

				// Attach handlers via .then()
				if _, thenErr := invokeThen(nextPromise, onFulfilled, onRejected); thenErr != nil {
					// Calling .then() itself threw (e.g. a thenable resolving to
					// itself). Per spec this should reject the result promise
					// (IfAbruptRejectPromise), not vanish - vm.Call leaves
					// vm.unwinding set on error for legitimate re-throw callers;
					// we're absorbing it into a rejection instead, so it must be
					// cleared or it leaks into whatever bytecode called this
					// static method.
					errVal := vm.NewString(thenErr.Error())
					if ee, ok := thenErr.(vm.ExceptionError); ok {
						errVal = ee.GetExceptionValue()
					}
					vmInstance.ClearUnwindingState()
					_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{errVal})
					return vm.Undefined, nil
				}
			}

			return vm.Undefined, nil
		})

		// Use the species constructor to create the result promise
		if constructor.IsCallable() {
			return vmInstance.Call(constructor, vm.Undefined, []vm.Value{executor})
		}
		return vmInstance.NewPromiseFromExecutor(executor)
	}))

	// Promise.allSettled(iterable)
	props.SetOwnNonEnumerable("allSettled", vm.NewNativeFunction(1, false, "allSettled", func(args []vm.Value) (vm.Value, error) {
		iterable := vm.Undefined
		if len(args) > 0 {
			iterable = args[0]
		}

		// Step 1-2: Let C be the this value. If Type(C) is not Object, throw TypeError.
		thisVal := vmInstance.GetThis()
		if !thisVal.IsObject() && !thisVal.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Promise.allSettled called on non-object")
		}
		constructor := getSpeciesConstructor(thisVal)

		// Convert iterable to array
		arr, err := vmInstance.IterableToArray(iterable)
		if err != nil {
			return vm.Undefined, vmInstance.NewTypeError("Promise.allSettled requires an iterable")
		}

		arrayObj := arr.AsArray()
		if arrayObj == nil {
			return vm.Undefined, vmInstance.NewTypeError("Promise.allSettled requires an iterable")
		}

		length := arrayObj.Length()
		if length == 0 {
			// Empty array resolves immediately to empty array
			return vmInstance.NewResolvedPromise(arr), nil
		}

		// Create a new promise that resolves when all promises settle
		capabilityGuard := promiseCapabilityGuard(vmInstance)
		executor := vm.NewNativeFunction(2, false, "executor", func(execArgs []vm.Value) (vm.Value, error) {
			resolve, reject, guardErr := capabilityGuard(execArgs)
			if guardErr != nil {
				return vm.Undefined, guardErr
			}

			// GetPromiseResolve(C) - per spec, if this fails, reject the promise
			promiseResolve, resolveErr := getPromiseResolve(constructor)
			if resolveErr != nil {
				errVal := vm.NewString(resolveErr.Error())
				if ee, ok := resolveErr.(vm.ExceptionError); ok {
					errVal = ee.GetExceptionValue()
				}
				vmInstance.ClearUnwindingState()
				_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{errVal})
				return vm.Undefined, nil
			}

			// Track results and completion count
			results := make([]vm.Value, length)
			remaining := length

			// Attach handlers to each promise
			for i := 0; i < length; i++ {
				idx := i // Capture index for closure
				promiseOrValue := arrayObj.Get(i)

				// Call C.resolve(promiseOrValue) per spec
				nextPromise, callErr := vmInstance.Call(promiseResolve, constructor, []vm.Value{promiseOrValue})
				if callErr != nil {
					errVal := vm.NewString(callErr.Error())
					if ee, ok := callErr.(vm.ExceptionError); ok {
						errVal = ee.GetExceptionValue()
					}
					vmInstance.ClearUnwindingState()
					_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{errVal})
					return vm.Undefined, nil
				}

				// Attach fulfillment/rejection handlers. Per ECMAScript
				// 27.2.4.2.1 (Promise.allSettled Resolve/Reject Element
				// Functions), the pair for one element shares a single
				// [[AlreadyCalled]] flag: nextPromise can be an arbitrary
				// thenable calling either handler more than once (or both),
				// and without this guard that would double-decrement
				// `remaining` and let results[idx] be overwritten after
				// settling.
				alreadyCalled := false
				onFulfilled := vm.NewNativeFunction(1, false, "onFulfilled", func(valueArgs []vm.Value) (vm.Value, error) {
					if alreadyCalled {
						return vm.Undefined, nil
					}
					alreadyCalled = true

					value := vm.Undefined
					if len(valueArgs) > 0 {
						value = valueArgs[0]
					}

					// Create { status: "fulfilled", value: ... } object
					resultObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					resultObj.SetOwnNonEnumerable("status", vm.NewString("fulfilled"))
					resultObj.SetOwnNonEnumerable("value", value)

					results[idx] = vm.NewValueFromPlainObject(resultObj)
					remaining--

					if remaining == 0 {
						// All promises settled - create result array
						resultArray := vmInstance.NewArrayFromSlice(results)
						_, _ = vmInstance.Call(resolve, vm.Undefined, []vm.Value{resultArray})
					}

					return vm.Undefined, nil
				})

				// Attach rejection handler
				onRejected := vm.NewNativeFunction(1, false, "onRejected", func(reasonArgs []vm.Value) (vm.Value, error) {
					if alreadyCalled {
						return vm.Undefined, nil
					}
					alreadyCalled = true

					reason := vm.Undefined
					if len(reasonArgs) > 0 {
						reason = reasonArgs[0]
					}

					// Create { status: "rejected", reason: ... } object
					resultObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
					resultObj.SetOwnNonEnumerable("status", vm.NewString("rejected"))
					resultObj.SetOwnNonEnumerable("reason", reason)

					results[idx] = vm.NewValueFromPlainObject(resultObj)
					remaining--

					if remaining == 0 {
						// All promises settled - create result array
						resultArray := vmInstance.NewArrayFromSlice(results)
						_, _ = vmInstance.Call(resolve, vm.Undefined, []vm.Value{resultArray})
					}

					return vm.Undefined, nil
				})

				// Attach handlers via .then()
				if _, thenErr := invokeThen(nextPromise, onFulfilled, onRejected); thenErr != nil {
					// Calling .then() itself threw (e.g. a thenable resolving to
					// itself). Per spec this should reject the result promise
					// (IfAbruptRejectPromise), not vanish - vm.Call leaves
					// vm.unwinding set on error for legitimate re-throw callers;
					// we're absorbing it into a rejection instead, so it must be
					// cleared or it leaks into whatever bytecode called this
					// static method.
					errVal := vm.NewString(thenErr.Error())
					if ee, ok := thenErr.(vm.ExceptionError); ok {
						errVal = ee.GetExceptionValue()
					}
					vmInstance.ClearUnwindingState()
					_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{errVal})
					return vm.Undefined, nil
				}
			}

			return vm.Undefined, nil
		})

		// Use the species constructor to create the result promise
		if constructor.IsCallable() {
			return vmInstance.Call(constructor, vm.Undefined, []vm.Value{executor})
		}
		return vmInstance.NewPromiseFromExecutor(executor)
	}))

	// Promise.try(callbackfn, ...args) - ES2025
	props.SetOwnNonEnumerable("try", vm.NewNativeFunction(1, true, "try", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 || !args[0].IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Promise.try requires a callable argument")
		}
		callbackfn := args[0]

		// Get extra arguments to pass to the callback
		var callArgs []vm.Value
		if len(args) > 1 {
			callArgs = args[1:]
		}

		// Create a new promise via executor
		capabilityGuard := promiseCapabilityGuard(vmInstance)
		executor := vm.NewNativeFunction(2, false, "executor", func(execArgs []vm.Value) (vm.Value, error) {
			resolve, reject, guardErr := capabilityGuard(execArgs)
			if guardErr != nil {
				return vm.Undefined, guardErr
			}

			// Call the callback synchronously
			result, err := vmInstance.Call(callbackfn, vm.Undefined, callArgs)
			if err != nil || vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				// Callback threw - reject with the error
				errVal := vm.Undefined
				if ee, ok := err.(vm.ExceptionError); ok {
					errVal = ee.GetExceptionValue()
				} else if err != nil {
					errVal = vm.NewString(err.Error())
				}
				vmInstance.ClearUnwindingState()
				_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{errVal})
				return vm.Undefined, nil
			}

			// Callback succeeded - resolve with the result
			_, _ = vmInstance.Call(resolve, vm.Undefined, []vm.Value{result})
			return vm.Undefined, nil
		})

		// Use the species constructor to create the result promise
		thisVal := vmInstance.GetThis()
		constructor := getSpeciesConstructor(thisVal)
		if constructor.IsCallable() {
			return vmInstance.Call(constructor, vm.Undefined, []vm.Value{executor})
		}
		return vmInstance.NewPromiseFromExecutor(executor)
	}))

	// Set constructor property on prototype
	promiseProto.SetOwnNonEnumerable("constructor", promiseCtor)

	// Register Promise constructor as global
	return ctx.DefineGlobal("Promise", promiseCtor)
}
