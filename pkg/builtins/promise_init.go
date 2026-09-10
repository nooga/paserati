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

// newAggregateError builds a real `new AggregateError(errors, message)` instance
// (so callers get a genuine `.errors` array and `instanceof Error`/`AggregateError`,
// per spec) by invoking the actual global constructor rather than faking one up
// as a plain string. Promise.any's "all promises rejected" rejection (both the
// empty-iterable case and the "all N settled as rejected" case) is the spec's
// only built-in producer of AggregateError, and used to reject with a bare
// string message with no `.errors` at all - see paserati#293, found via real
// npm `undici` code that does `err instanceof AggregateError && err.errors.some(...)`.
// Falls back to a plain Error if the global was somehow removed/shadowed, so
// this can never itself throw.
func newAggregateError(vmInstance *vm.VM, errors vm.Value, message string) vm.Value {
	if ctor, ok := vmInstance.GetGlobal("AggregateError"); ok && ctor.IsCallable() {
		if inst, err := vmInstance.Construct(ctor, []vm.Value{errors, vm.NewString(message)}); err == nil {
			return inst
		}
	}
	return vm.NewString(message)
}

func (p *PromiseInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create Promise.prototype inheriting from Object.prototype
	promiseProto := vm.NewObject(objectProto).AsPlainObject()

	// The intrinsic %Promise% constructor, needed by the prototype methods'
	// SpeciesConstructor step below but only built further down; the closures
	// capture the variable, which is assigned as soon as it exists.
	var intrinsicPromise vm.Value

	// speciesThen is PerformPromiseThen with the result promise built by
	// SpeciesConstructor(promise, %Promise%) per ES 27.2.5.4 step 3 - which is
	// what makes `class P extends Promise {}` have p.then(...) return a P.
	// Only the prototype methods do this; the static combinators build their
	// capability from `this` directly (see Promise.all).
	speciesThen := func(thisVal vm.Value, onFulfilled, onRejected vm.Value) (vm.Value, error) {
		ctor, err := promiseSpeciesConstructor(vmInstance, thisVal, intrinsicPromise)
		if err != nil {
			return vm.Undefined, err
		}
		if ctor.Type() == vm.TypeUndefined || ctor.Is(intrinsicPromise) {
			// Intrinsic %Promise%: keep the direct path, which skips the
			// executor round-trip through Construct.
			return vmInstance.PromiseThen(thisVal, onFulfilled, onRejected)
		}
		return vmInstance.PromiseThenWith(thisVal, onFulfilled, onRejected, func(executor vm.Value) (vm.Value, error) {
			return vmInstance.Construct(ctor, []vm.Value{executor})
		})
	}

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

		return speciesThen(thisVal, onFulfilled, onRejected)
	}))

	// Promise.prototype.catch(onRejected)
	promiseProto.SetOwnNonEnumerable("catch", vm.NewNativeFunction(1, false, "catch", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		onRejected := vm.Undefined
		if len(args) > 0 {
			onRejected = args[0]
		}

		// catch(onRejected) is equivalent to then(undefined, onRejected)
		return speciesThen(thisVal, vm.Undefined, onRejected)
	}))

	// Promise.prototype.finally(onFinally) - ES 27.2.5.3.
	//
	// The previous implementation registered ONE wrapper for both the fulfill
	// and the reject reaction and returned its argument, which turned every
	// rejection into a fulfillment carrying the reason, and it discarded
	// onFinally's return value entirely instead of awaiting it. Per spec the
	// two reactions are distinct: each calls onFinally, wraps its result with
	// PromiseResolve(C, result) - so a thenable is awaited - and only then
	// replays the original outcome, re-throwing in the reject case.
	promiseProto.SetOwnNonEnumerable("finally", vm.NewNativeFunction(1, false, "finally", func(args []vm.Value) (vm.Value, error) {
		// Step 2: If promise is not an Object, throw a TypeError.
		thisVal := vmInstance.GetThis()
		if !thisVal.IsObject() && !thisVal.IsCallable() && thisVal.Type() != vm.TypePromise {
			return vm.Undefined, vmInstance.NewTypeError("Promise.prototype.finally called on a non-object")
		}
		onFinally := vm.Undefined
		if len(args) > 0 {
			onFinally = args[0]
		}

		// Step 3: C = SpeciesConstructor(promise, %Promise%).
		ctor, err := promiseSpeciesConstructor(vmInstance, thisVal, intrinsicPromise)
		if err != nil {
			return vm.Undefined, err
		}

		// Step 5: a non-callable onFinally is installed as both handlers
		// unchanged, so `then` applies its own "not callable" pass-through.
		if !onFinally.IsCallable() {
			return invokeThen(vmInstance, thisVal, onFinally, onFinally)
		}

		// Step 6: thenFinally / catchFinally. replay is what runs after
		// PromiseResolve(C, onFinally()) settles: return the original value,
		// or re-throw the original reason.
		// Both handlers are anonymous per spec (built-ins/Promise/prototype/
		// finally/invokes-then-with-function.js asserts name === "").
		makeHandler := func(rethrow bool) vm.Value {
			return vm.NewNativeFunction(1, false, "", func(handlerArgs []vm.Value) (vm.Value, error) {
				outcome := vm.Undefined
				if len(handlerArgs) > 0 {
					outcome = handlerArgs[0]
				}
				result, callErr := vmInstance.Call(onFinally, vm.Undefined, nil)
				if callErr != nil {
					return vm.Undefined, callErr
				}
				wrapped, resolveErr := promiseResolveWith(vmInstance, ctor, result)
				if resolveErr != nil {
					return vm.Undefined, resolveErr
				}
				replay := vm.NewNativeFunction(0, false, "", func([]vm.Value) (vm.Value, error) {
					if rethrow {
						return vm.Undefined, vmInstance.NewExceptionError(outcome)
					}
					return outcome, nil
				})
				return invokeThen(vmInstance, wrapped, replay, vm.Undefined)
			})
		}

		// Step 7: Invoke(promise, "then", thenFinally, catchFinally).
		return invokeThen(vmInstance, thisVal, makeHandler(false), makeHandler(true))
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

	// Publish the intrinsic to the prototype methods' SpeciesConstructor step
	// (declared above, before Promise.prototype's methods were installed).
	intrinsicPromise = promiseCtor

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

		// Step 3: PromiseResolve(C, value). This used to return an
		// already-fulfilled intrinsic promise, which got two things wrong: it
		// ignored `this`, so Promise.resolve.call(SubPromise, v) produced a
		// plain Promise, and it fulfilled WITH a thenable instead of
		// assimilating it, because it never went through a resolve function.
		return promiseResolveWith(vmInstance, thisVal, value)
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

	// Promise[Symbol.species] - an accessor whose getter returns 'this', so a
	// subclass inherits it and answers itself. This used to be a plain data
	// property holding Promise, which made `class P extends Promise {}` report
	// Promise rather than P.
	defineSpeciesAccessor(vmInstance, props)

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

		// Step 1-2: Let C be the this value. NewPromiseCapability(C) (spec step
		// 6-7, run before the iterable is even looked at) requires
		// IsConstructor(C) and throws TypeError synchronously otherwise - a
		// plain IsObject()/IsCallable() check lets a non-constructor callable
		// like `eval` through, and this must throw here rather than surface
		// later as a rejected promise (paserati#293 regression check:
		// `Promise.all.call(eval)` used to throw synchronously only by
		// accident, because converting the missing iterable argument failed
		// too and *that* error path threw synchronously; now that iterable
		// conversion failures correctly reject the promise instead - see
		// IfAbruptRejectPromise below - this check needs to actually be here).
		thisVal := vmInstance.GetThis()
		if !vmInstance.IsConstructor(thisVal) {
			return vm.Undefined, vmInstance.NewTypeError("Promise.all called on non-constructor")
		}
		// Per spec the static combinators do NewPromiseCapability(C) on the
		// `this` value directly - they must NOT read C[Symbol.species]
		// (built-ins/Promise/{all,allSettled,any,race}/species-get-error.js all
		// install a throwing species getter and require it never runs). Only
		// Promise.prototype.then/finally use SpeciesConstructor.
		constructor := thisVal

		// Convert iterable to array (before promise creation per spec)
		arr, err := vmInstance.IterableToArray(iterable)
		if err != nil {
			// Per spec (IfAbruptRejectPromise), a failure here - GetIterator
			// throwing, iterator.next() throwing, or an ordinary getter access
			// like result.value throwing mid-iteration - rejects the result
			// promise with that failure's actual value; it must not escape as
			// a synchronous throw out of Promise.all itself (paserati#293:
			// IterableToArray now correctly propagates such errors instead of
			// silently swallowing them, so this needs to actually honor
			// IfAbruptRejectPromise instead of only handling "not iterable").
			// vm.Call (inside IterableToArray) leaves vm.unwinding set on
			// error for legitimate re-throw callers; absorbing that error
			// into a rejection here instead means it must be cleared, or it
			// leaks into whatever bytecode called this static method (see
			// the matching invoke-then-error-close style handlers below).
			vmInstance.ClearUnwindingState()
			return vmInstance.NewRejectedPromise(exceptionValue(vmInstance, err)), nil
		}

		arrayObj := arr.AsArray()
		if arrayObj == nil {
			return vmInstance.NewRejectedPromise(exceptionValue(vmInstance, vmInstance.NewTypeError("Promise.all requires an iterable"))), nil
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
			return vmInstance.Construct(constructor, []vm.Value{executor})
		}
		return vmInstance.NewPromiseFromExecutor(executor)
	}))

	// Promise.race(iterable)
	props.SetOwnNonEnumerable("race", vm.NewNativeFunction(1, false, "race", func(args []vm.Value) (vm.Value, error) {
		iterable := vm.Undefined
		if len(args) > 0 {
			iterable = args[0]
		}

		// See the matching comment in Promise.all: NewPromiseCapability(C)
		// requires IsConstructor(C), checked synchronously before the
		// iterable is touched.
		thisVal := vmInstance.GetThis()
		if !vmInstance.IsConstructor(thisVal) {
			return vm.Undefined, vmInstance.NewTypeError("Promise.race called on non-constructor")
		}
		// NewPromiseCapability(C) on `this` directly, no species read - see
		// Promise.all above.
		constructor := thisVal

		// Convert iterable to array
		arr, err := vmInstance.IterableToArray(iterable)
		if err != nil {
			// See the matching comment in Promise.all: IfAbruptRejectPromise
			// means this rejects the result promise with the real value,
			// rather than throwing synchronously out of Promise.race itself.
			// vm.Call (inside IterableToArray) leaves vm.unwinding set on
			// error for legitimate re-throw callers; absorbing that error
			// into a rejection here instead means it must be cleared, or it
			// leaks into whatever bytecode called this static method (see
			// the matching invoke-then-error-close style handlers below).
			vmInstance.ClearUnwindingState()
			return vmInstance.NewRejectedPromise(exceptionValue(vmInstance, err)), nil
		}

		arrayObj := arr.AsArray()
		if arrayObj == nil {
			return vmInstance.NewRejectedPromise(exceptionValue(vmInstance, vmInstance.NewTypeError("Promise.race requires an iterable"))), nil
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
			return vmInstance.Construct(constructor, []vm.Value{executor})
		}
		return vmInstance.NewPromiseFromExecutor(executor)
	}))

	// Promise.any(iterable)
	props.SetOwnNonEnumerable("any", vm.NewNativeFunction(1, false, "any", func(args []vm.Value) (vm.Value, error) {
		iterable := vm.Undefined
		if len(args) > 0 {
			iterable = args[0]
		}

		// See the matching comment in Promise.all: NewPromiseCapability(C)
		// requires IsConstructor(C), checked synchronously before the
		// iterable is touched.
		thisVal := vmInstance.GetThis()
		if !vmInstance.IsConstructor(thisVal) {
			return vm.Undefined, vmInstance.NewTypeError("Promise.any called on non-constructor")
		}
		// NewPromiseCapability(C) on `this` directly, no species read - see
		// Promise.all above.
		constructor := thisVal

		// Convert iterable to array
		arr, err := vmInstance.IterableToArray(iterable)
		if err != nil {
			// See the matching comment in Promise.all: IfAbruptRejectPromise
			// means this rejects the result promise with the real value,
			// rather than throwing synchronously out of Promise.any itself.
			// vm.Call (inside IterableToArray) leaves vm.unwinding set on
			// error for legitimate re-throw callers; absorbing that error
			// into a rejection here instead means it must be cleared, or it
			// leaks into whatever bytecode called this static method (see
			// the matching invoke-then-error-close style handlers below).
			vmInstance.ClearUnwindingState()
			return vmInstance.NewRejectedPromise(exceptionValue(vmInstance, err)), nil
		}

		arrayObj := arr.AsArray()
		if arrayObj == nil {
			return vmInstance.NewRejectedPromise(exceptionValue(vmInstance, vmInstance.NewTypeError("Promise.any requires an iterable"))), nil
		}

		length := arrayObj.Length()
		if length == 0 {
			// Empty array - reject immediately with AggregateError
			aggErr := newAggregateError(vmInstance, vm.NewArray(), "All promises were rejected")
			return vmInstance.NewRejectedPromise(aggErr), nil
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
						errorsArray := vmInstance.NewArrayFromSlice(errors)
						aggErr := newAggregateError(vmInstance, errorsArray, "All promises were rejected")
						_, _ = vmInstance.Call(reject, vm.Undefined, []vm.Value{aggErr})
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
			return vmInstance.Construct(constructor, []vm.Value{executor})
		}
		return vmInstance.NewPromiseFromExecutor(executor)
	}))

	// Promise.allSettled(iterable)
	props.SetOwnNonEnumerable("allSettled", vm.NewNativeFunction(1, false, "allSettled", func(args []vm.Value) (vm.Value, error) {
		iterable := vm.Undefined
		if len(args) > 0 {
			iterable = args[0]
		}

		// See the matching comment in Promise.all: NewPromiseCapability(C)
		// requires IsConstructor(C), checked synchronously before the
		// iterable is touched.
		thisVal := vmInstance.GetThis()
		if !vmInstance.IsConstructor(thisVal) {
			return vm.Undefined, vmInstance.NewTypeError("Promise.allSettled called on non-constructor")
		}
		// NewPromiseCapability(C) on `this` directly, no species read - see
		// Promise.all above.
		constructor := thisVal

		// Convert iterable to array
		arr, err := vmInstance.IterableToArray(iterable)
		if err != nil {
			// See the matching comment in Promise.all: IfAbruptRejectPromise
			// means this rejects the result promise with the real value,
			// rather than throwing synchronously out of Promise.allSettled.
			// vm.Call (inside IterableToArray) leaves vm.unwinding set on
			// error for legitimate re-throw callers; absorbing that error
			// into a rejection here instead means it must be cleared, or it
			// leaks into whatever bytecode called this static method (see
			// the matching invoke-then-error-close style handlers below).
			vmInstance.ClearUnwindingState()
			return vmInstance.NewRejectedPromise(exceptionValue(vmInstance, err)), nil
		}

		arrayObj := arr.AsArray()
		if arrayObj == nil {
			return vmInstance.NewRejectedPromise(exceptionValue(vmInstance, vmInstance.NewTypeError("Promise.allSettled requires an iterable"))), nil
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
			return vmInstance.Construct(constructor, []vm.Value{executor})
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
		// NewPromiseCapability(C) on `this` directly, no species read - see
		// Promise.all above.
		constructor := thisVal
		if constructor.IsCallable() {
			return vmInstance.Construct(constructor, []vm.Value{executor})
		}
		return vmInstance.NewPromiseFromExecutor(executor)
	}))

	// Set constructor property on prototype
	promiseProto.SetOwnNonEnumerable("constructor", promiseCtor)

	// Register Promise constructor as global
	return ctx.DefineGlobal("Promise", promiseCtor)
}

// promiseSpeciesConstructor implements SpeciesConstructor(O, %Promise%) for
// Promise.prototype.then/catch/finally (ES 27.2.5.4 step 3 via 7.3.23).
//
//  1. C = O.constructor; if undefined, use the default.
//  2. If C is not an Object, throw a TypeError.
//  3. S = C[Symbol.species]; if undefined or null, use the default.
//  4. If S is a constructor, return it; otherwise throw a TypeError.
//
// Returns the default (the intrinsic %Promise%) for every case the spec routes
// there, so callers can compare against it to take the fast path.
func promiseSpeciesConstructor(vmInstance *vm.VM, promise vm.Value, defaultCtor vm.Value) (vm.Value, error) {
	ctor, err := vmInstance.GetProperty(promise, "constructor")
	if err != nil {
		return vm.Undefined, err
	}
	if ctor.Type() == vm.TypeUndefined {
		return defaultCtor, nil
	}
	if !ctor.IsObject() && !ctor.IsCallable() {
		return vm.Undefined, vmInstance.NewTypeError("Promise.prototype.then: constructor property is not an object")
	}
	species, found, err := vmInstance.GetSymbolPropertyWithGetter(ctor, vmInstance.SymbolSpecies)
	if err != nil {
		return vm.Undefined, err
	}
	if !found || species.Type() == vm.TypeUndefined || species.Type() == vm.TypeNull {
		return defaultCtor, nil
	}
	if !vmInstance.IsConstructor(species) {
		return vm.Undefined, vmInstance.NewTypeError("Promise.prototype.then: @@species is not a constructor")
	}
	return species, nil
}

// invokeThen implements Invoke(promise, "then", args) - it reads the actual
// `then` property off the value rather than calling the intrinsic directly, so
// a subclass's (or a thenable's) own override is honored, which is what
// Promise.prototype.finally's spec steps require.
func invokeThen(vmInstance *vm.VM, promise vm.Value, onFulfilled, onRejected vm.Value) (vm.Value, error) {
	then, err := vmInstance.GetProperty(promise, "then")
	if err != nil {
		return vm.Undefined, err
	}
	if !then.IsCallable() {
		return vm.Undefined, vmInstance.NewTypeError("Promise.prototype.finally: `then` is not callable")
	}
	return vmInstance.Call(then, promise, []vm.Value{onFulfilled, onRejected})
}

// promiseResolveWith implements PromiseResolve(C, x) (ES 27.2.4.7.1): a promise
// whose own constructor is already C passes through untouched; anything else -
// a plain value, a foreign promise, or a thenable - is fed through a fresh
// NewPromiseCapability(C)'s resolve function, which is what assimilates a
// thenable rather than fulfilling with it.
//
// Note this deliberately does NOT go through C.resolve: the spec operation
// builds the capability directly, and routing through C.resolve would recurse
// forever when C is %Promise% (whose resolve is this function's only caller).
func promiseResolveWith(vmInstance *vm.VM, ctor vm.Value, x vm.Value) (vm.Value, error) {
	if x.Type() == vm.TypePromise {
		xCtor, err := vmInstance.GetProperty(x, "constructor")
		if err != nil {
			return vm.Undefined, err
		}
		if xCtor.Is(ctor) {
			return x, nil
		}
	}

	capResolve := vm.Undefined
	guard := promiseCapabilityGuard(vmInstance)
	executor := vm.NewNativeFunction(2, false, "", func(execArgs []vm.Value) (vm.Value, error) {
		resolve, _, guardErr := guard(execArgs)
		if guardErr != nil {
			return vm.Undefined, guardErr
		}
		capResolve = resolve
		return vm.Undefined, nil
	})
	promise, err := vmInstance.Construct(ctor, []vm.Value{executor})
	if err != nil {
		return vm.Undefined, err
	}
	if !capResolve.IsCallable() {
		return vm.Undefined, vmInstance.NewTypeError("PromiseResolve: capability's resolve function is not callable")
	}
	if _, err := vmInstance.Call(capResolve, vm.Undefined, []vm.Value{x}); err != nil {
		return vm.Undefined, err
	}
	return promise, nil
}
