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
		WithProperty("withResolvers", types.NewSimpleFunction(
			[]types.Type{},
			types.Any, // { promise, resolve, reject }
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

	arg := func(args []vm.Value, i int) vm.Value {
		if i < len(args) {
			return args[i]
		}
		return vm.Undefined
	}

	// Promise.prototype inherits from Object.prototype
	promiseProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// The intrinsic %Promise% constructor; the prototype methods below are
	// installed before it exists and capture this variable.
	var intrinsicPromise vm.Value

	// Promise.prototype.then(onFulfilled, onRejected) - 27.2.5.4
	promiseProto.SetOwnNonEnumerable("then", vm.NewNativeFunction(2, false, "then", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() != vm.TypePromise {
			return vm.Undefined, vmInstance.NewTypeError("Promise.prototype.then called on incompatible receiver")
		}
		c, err := promiseSpeciesConstructor(vmInstance, thisVal, intrinsicPromise)
		if err != nil {
			return vm.Undefined, err
		}
		pc, err := newPromiseCapability(vmInstance, c)
		if err != nil {
			return vm.Undefined, err
		}
		vmInstance.PerformPromiseThen(thisVal.AsPromise(), arg(args, 0), arg(args, 1), pc.resolve, pc.reject)
		return pc.promise, nil
	}))

	// Promise.prototype.catch(onRejected) - 27.2.5.1: Invoke(this, "then").
	promiseProto.SetOwnNonEnumerable("catch", vm.NewNativeFunction(1, false, "catch", func(args []vm.Value) (vm.Value, error) {
		return invoke(vmInstance, vmInstance.GetThis(), "then", vm.Undefined, arg(args, 0))
	}))

	// Promise.prototype.finally(onFinally) - 27.2.5.3
	promiseProto.SetOwnNonEnumerable("finally", vm.NewNativeFunction(1, false, "finally", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if !thisVal.IsObject() && !thisVal.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Promise.prototype.finally called on a non-object")
		}
		onFinally := arg(args, 0)
		c, err := promiseSpeciesConstructor(vmInstance, thisVal, intrinsicPromise)
		if err != nil {
			return vm.Undefined, err
		}
		if !onFinally.IsCallable() {
			return invoke(vmInstance, thisVal, "then", onFinally, onFinally)
		}
		// thenFinally / catchFinally: call onFinally, wait for
		// PromiseResolve(C, its result), then replay the original outcome.
		makeHandler := func(rethrow bool) vm.Value {
			return vm.NewNativeFunction(1, false, "", func(handlerArgs []vm.Value) (vm.Value, error) {
				outcome := arg(handlerArgs, 0)
				result, err := vmInstance.Call(onFinally, vm.Undefined, nil)
				if err != nil {
					return vm.Undefined, err
				}
				wrapped, err := promiseResolve(vmInstance, c, result)
				if err != nil {
					return vm.Undefined, err
				}
				replay := vm.NewNativeFunction(0, false, "", func([]vm.Value) (vm.Value, error) {
					if rethrow {
						return vm.Undefined, vmInstance.NewExceptionError(outcome)
					}
					return outcome, nil
				})
				return invoke(vmInstance, wrapped, "then", replay)
			})
		}
		return invoke(vmInstance, thisVal, "then", makeHandler(false), makeHandler(true))
	}))

	// Promise.prototype[@@toStringTag] = "Promise"
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		wFalse, eFalse, cTrue := false, false, true
		promiseProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Promise"),
			&wFalse, &eFalse, &cTrue,
		)
	}

	vmInstance.PromisePrototype = vm.NewValueFromPlainObject(promiseProto)

	// Promise(executor) - 27.2.3.1
	promiseCtor := vm.NewConstructorWithProps(1, true, "Promise", func(args []vm.Value) (vm.Value, error) {
		newTarget := vmInstance.GetNewTarget()
		if newTarget.Type() == vm.TypeUndefined {
			return vm.Undefined, vmInstance.NewTypeError("Promise constructor cannot be invoked without 'new'")
		}
		executor := arg(args, 0)
		if !executor.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Promise resolver " + executor.TypeName() + " is not a function")
		}
		// OrdinaryCreateFromConstructor(NewTarget, "%Promise.prototype%")
		if _, err := vmInstance.GetPrototypeFromConstructor(newTarget, "%PromisePrototype%"); err != nil {
			return vm.Undefined, err
		}
		return vmInstance.NewPromiseFromExecutor(executor)
	})
	intrinsicPromise = promiseCtor
	vmInstance.PromiseConstructor = promiseCtor

	props := promiseCtor.AsNativeFunctionWithProps().Properties
	props.DefineFixedProperty("prototype", vmInstance.PromisePrototype)

	requireObjectThis := func(method string) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if !thisVal.IsObject() && !thisVal.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Promise." + method + " called on non-object")
		}
		return thisVal, nil
	}

	// Promise.resolve(x) - 27.2.4.7
	props.SetOwnNonEnumerable("resolve", vm.NewNativeFunction(1, false, "resolve", func(args []vm.Value) (vm.Value, error) {
		c, err := requireObjectThis("resolve")
		if err != nil {
			return vm.Undefined, err
		}
		return promiseResolve(vmInstance, c, arg(args, 0))
	}))

	// Promise.reject(r) - 27.2.4.6
	props.SetOwnNonEnumerable("reject", vm.NewNativeFunction(1, false, "reject", func(args []vm.Value) (vm.Value, error) {
		pc, err := newPromiseCapability(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		if _, err := vmInstance.Call(pc.reject, vm.Undefined, []vm.Value{arg(args, 0)}); err != nil {
			return vm.Undefined, err
		}
		return pc.promise, nil
	}))

	// Promise.withResolvers() - 27.2.4.9
	props.SetOwnNonEnumerable("withResolvers", vm.NewNativeFunction(0, false, "withResolvers", func(args []vm.Value) (vm.Value, error) {
		pc, err := newPromiseCapability(vmInstance, vmInstance.GetThis())
		if err != nil {
			return vm.Undefined, err
		}
		obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		obj.SetOwn("promise", pc.promise)
		obj.SetOwn("resolve", pc.resolve)
		obj.SetOwn("reject", pc.reject)
		return vm.NewValueFromPlainObject(obj), nil
	}))

	defineSpeciesAccessor(vmInstance, props)

	// Promise.all / allSettled / any / race - 27.2.4.1-5
	props.SetOwnNonEnumerable("all", vm.NewNativeFunction(1, false, "all", func(args []vm.Value) (vm.Value, error) {
		c := vmInstance.GetThis()
		return promiseCombinator(vmInstance, c, arg(args, 0), performPromiseAll(vmInstance, c, false))
	}))
	props.SetOwnNonEnumerable("allSettled", vm.NewNativeFunction(1, false, "allSettled", func(args []vm.Value) (vm.Value, error) {
		c := vmInstance.GetThis()
		return promiseCombinator(vmInstance, c, arg(args, 0), performPromiseAll(vmInstance, c, true))
	}))
	props.SetOwnNonEnumerable("any", vm.NewNativeFunction(1, false, "any", func(args []vm.Value) (vm.Value, error) {
		c := vmInstance.GetThis()
		return promiseCombinator(vmInstance, c, arg(args, 0), performPromiseAny(vmInstance, c))
	}))
	props.SetOwnNonEnumerable("race", vm.NewNativeFunction(1, false, "race", func(args []vm.Value) (vm.Value, error) {
		c := vmInstance.GetThis()
		return promiseCombinator(vmInstance, c, arg(args, 0), performPromiseRace(vmInstance, c))
	}))

	// Promise.try(callbackfn, ...args) - 27.2.4.8
	props.SetOwnNonEnumerable("try", vm.NewNativeFunction(1, true, "try", func(args []vm.Value) (vm.Value, error) {
		c, err := requireObjectThis("try")
		if err != nil {
			return vm.Undefined, err
		}
		pc, err := newPromiseCapability(vmInstance, c)
		if err != nil {
			return vm.Undefined, err
		}
		var callArgs []vm.Value
		if len(args) > 1 {
			callArgs = args[1:]
		}
		result, err := vmInstance.Call(arg(args, 0), vm.Undefined, callArgs)
		if err != nil {
			return pc.rejectWith(vmInstance, err)
		}
		if _, err := vmInstance.Call(pc.resolve, vm.Undefined, []vm.Value{result}); err != nil {
			return vm.Undefined, err
		}
		return pc.promise, nil
	}))

	promiseProto.SetOwnNonEnumerable("constructor", promiseCtor)

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

