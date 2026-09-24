package builtins

import "github.com/nooga/paserati/pkg/vm"

// Promise abstract operations (ECMA-262 27.2.1 - 27.2.4) shared by the
// Promise constructor's static methods and Promise.prototype.

// promiseCapability is a PromiseCapability Record.
type promiseCapability struct {
	promise vm.Value
	resolve vm.Value
	reject  vm.Value
}

// newPromiseCapability is NewPromiseCapability(C). For the intrinsic
// %Promise% the capability is built directly, which is unobservable.
func newPromiseCapability(vmi *vm.VM, c vm.Value) (*promiseCapability, error) {
	if c.Is(vmi.PromiseConstructor) {
		p := vmi.NewPendingPromise()
		resolve, reject := vmi.CreateResolvingFunctions(p.AsPromise())
		return &promiseCapability{promise: p, resolve: resolve, reject: reject}, nil
	}
	if !vmi.IsConstructor(c) {
		return nil, vmi.NewTypeError("Promise capability constructor is not a constructor")
	}
	capResolve, capReject := vm.Undefined, vm.Undefined
	executor := vm.NewNativeFunction(2, false, "", func(args []vm.Value) (vm.Value, error) {
		if capResolve.Type() != vm.TypeUndefined {
			return vm.Undefined, vmi.NewTypeError("Promise executor has already been invoked with a resolve function")
		}
		if capReject.Type() != vm.TypeUndefined {
			return vm.Undefined, vmi.NewTypeError("Promise executor has already been invoked with a reject function")
		}
		if len(args) > 0 {
			capResolve = args[0]
		}
		if len(args) > 1 {
			capReject = args[1]
		}
		return vm.Undefined, nil
	})
	promise, err := vmi.Construct(c, []vm.Value{executor})
	if err != nil {
		return nil, err
	}
	if !capResolve.IsCallable() {
		return nil, vmi.NewTypeError("Promise resolve function is not callable")
	}
	if !capReject.IsCallable() {
		return nil, vmi.NewTypeError("Promise reject function is not callable")
	}
	return &promiseCapability{promise: promise, resolve: capResolve, reject: capReject}, nil
}

// rejectWith is IfAbruptRejectPromise: reject the capability with err's
// thrown value and return its promise.
func (pc *promiseCapability) rejectWith(vmi *vm.VM, err error) (vm.Value, error) {
	vmi.ClearUnwindingState()
	if _, callErr := vmi.Call(pc.reject, vm.Undefined, []vm.Value{vmi.ExceptionValueFromError(err)}); callErr != nil {
		return vm.Undefined, callErr
	}
	return pc.promise, nil
}

// promiseResolve is PromiseResolve(C, x).
func promiseResolve(vmi *vm.VM, c vm.Value, x vm.Value) (vm.Value, error) {
	if x.Type() == vm.TypePromise {
		xCtor, err := vmi.GetProperty(x, "constructor")
		if err != nil {
			return vm.Undefined, err
		}
		if xCtor.Is(c) {
			return x, nil
		}
	}
	pc, err := newPromiseCapability(vmi, c)
	if err != nil {
		return vm.Undefined, err
	}
	if _, err := vmi.Call(pc.resolve, vm.Undefined, []vm.Value{x}); err != nil {
		return vm.Undefined, err
	}
	return pc.promise, nil
}

// invoke is Invoke(v, name, args).
func invoke(vmi *vm.VM, v vm.Value, name string, args ...vm.Value) (vm.Value, error) {
	fn, err := vmi.GetProperty(v, name)
	if err != nil {
		return vm.Undefined, err
	}
	if !fn.IsCallable() {
		return vm.Undefined, vmi.NewTypeError("'" + name + "' is not a function")
	}
	return vmi.Call(fn, v, args)
}

// getPromiseResolve is GetPromiseResolve(C).
func getPromiseResolve(vmi *vm.VM, c vm.Value) (vm.Value, error) {
	resolve, err := vmi.GetProperty(c, "resolve")
	if err != nil {
		return vm.Undefined, err
	}
	if !resolve.IsCallable() {
		return vm.Undefined, vmi.NewTypeError("Promise resolve is not a function")
	}
	return resolve, nil
}

// promiseCombinator runs the shared shape of Promise.all/allSettled/any/race
// (27.2.4.1 etc.): NewPromiseCapability(C), GetPromiseResolve(C),
// GetIterator(iterable), then perform, closing the iterator and rejecting
// the capability if perform completes abruptly.
func promiseCombinator(vmi *vm.VM, c vm.Value, iterable vm.Value,
	perform func(rec *iteratorRecord, pc *promiseCapability, resolveFn vm.Value) (vm.Value, error)) (vm.Value, error) {
	pc, err := newPromiseCapability(vmi, c)
	if err != nil {
		return vm.Undefined, err
	}
	resolveFn, err := getPromiseResolve(vmi, c)
	if err != nil {
		return pc.rejectWith(vmi, err)
	}
	rec, err := getIterator(vmi, iterable)
	if err != nil {
		return pc.rejectWith(vmi, err)
	}
	result, err := perform(rec, pc, resolveFn)
	if err != nil {
		if !rec.done {
			vmi.ClearUnwindingState()
			err = iteratorClose(vmi, rec.iter, err)
		}
		return pc.rejectWith(vmi, err)
	}
	return result, nil
}

// performPromiseAll is PerformPromiseAll (settled=false) and
// PerformPromiseAllSettled (settled=true).
func performPromiseAll(vmi *vm.VM, c vm.Value, settled bool) func(*iteratorRecord, *promiseCapability, vm.Value) (vm.Value, error) {
	return func(rec *iteratorRecord, pc *promiseCapability, resolveFn vm.Value) (vm.Value, error) {
		var values []vm.Value
		remaining := 1
		finish := func() error {
			remaining--
			if remaining == 0 {
				_, err := vmi.Call(pc.resolve, vm.Undefined, []vm.Value{vmi.NewArrayFromSlice(append([]vm.Value(nil), values...))})
				return err
			}
			return nil
		}
		for index := 0; ; index++ {
			next, done, err := iteratorStepValue(vmi, rec)
			if err != nil {
				return vm.Undefined, err
			}
			if done {
				if err := finish(); err != nil {
					return vm.Undefined, err
				}
				return pc.promise, nil
			}
			values = append(values, vm.Undefined)
			nextPromise, err := vmi.Call(resolveFn, c, []vm.Value{next})
			if err != nil {
				return vm.Undefined, err
			}
			i := index
			alreadyCalled := false
			element := func(build func(vm.Value) vm.Value) vm.Value {
				return vm.NewNativeFunction(1, false, "", func(args []vm.Value) (vm.Value, error) {
					if alreadyCalled {
						return vm.Undefined, nil
					}
					alreadyCalled = true
					x := vm.Undefined
					if len(args) > 0 {
						x = args[0]
					}
					values[i] = build(x)
					return vm.Undefined, finish()
				})
			}
			var onFulfilled, onRejected vm.Value
			if settled {
				onFulfilled = element(func(x vm.Value) vm.Value { return settledRecord(vmi, "fulfilled", "value", x) })
				onRejected = element(func(x vm.Value) vm.Value { return settledRecord(vmi, "rejected", "reason", x) })
			} else {
				onFulfilled = element(func(x vm.Value) vm.Value { return x })
				onRejected = pc.reject
			}
			remaining++
			if _, err := invoke(vmi, nextPromise, "then", onFulfilled, onRejected); err != nil {
				return vm.Undefined, err
			}
		}
	}
}

func settledRecord(vmi *vm.VM, status, key string, x vm.Value) vm.Value {
	obj := vm.NewObject(vmi.ObjectPrototype).AsPlainObject()
	obj.SetOwn("status", vm.NewString(status))
	obj.SetOwn(key, x)
	return vm.NewValueFromPlainObject(obj)
}

// performPromiseAny is PerformPromiseAny.
func performPromiseAny(vmi *vm.VM, c vm.Value) func(*iteratorRecord, *promiseCapability, vm.Value) (vm.Value, error) {
	return func(rec *iteratorRecord, pc *promiseCapability, resolveFn vm.Value) (vm.Value, error) {
		var errs []vm.Value
		remaining := 1
		aggregate := func() vm.Value {
			return newAggregateError(vmi, vmi.NewArrayFromSlice(append([]vm.Value(nil), errs...)), "All promises were rejected")
		}
		for index := 0; ; index++ {
			next, done, err := iteratorStepValue(vmi, rec)
			if err != nil {
				return vm.Undefined, err
			}
			if done {
				remaining--
				if remaining == 0 {
					return vm.Undefined, vmi.NewExceptionError(aggregate())
				}
				return pc.promise, nil
			}
			errs = append(errs, vm.Undefined)
			nextPromise, err := vmi.Call(resolveFn, c, []vm.Value{next})
			if err != nil {
				return vm.Undefined, err
			}
			i := index
			alreadyCalled := false
			onRejected := vm.NewNativeFunction(1, false, "", func(args []vm.Value) (vm.Value, error) {
				if alreadyCalled {
					return vm.Undefined, nil
				}
				alreadyCalled = true
				x := vm.Undefined
				if len(args) > 0 {
					x = args[0]
				}
				errs[i] = x
				remaining--
				if remaining == 0 {
					return vmi.Call(pc.reject, vm.Undefined, []vm.Value{aggregate()})
				}
				return vm.Undefined, nil
			})
			remaining++
			if _, err := invoke(vmi, nextPromise, "then", pc.resolve, onRejected); err != nil {
				return vm.Undefined, err
			}
		}
	}
}

// performPromiseRace is PerformPromiseRace.
func performPromiseRace(vmi *vm.VM, c vm.Value) func(*iteratorRecord, *promiseCapability, vm.Value) (vm.Value, error) {
	return func(rec *iteratorRecord, pc *promiseCapability, resolveFn vm.Value) (vm.Value, error) {
		for {
			next, done, err := iteratorStepValue(vmi, rec)
			if err != nil {
				return vm.Undefined, err
			}
			if done {
				return pc.promise, nil
			}
			nextPromise, err := vmi.Call(resolveFn, c, []vm.Value{next})
			if err != nil {
				return vm.Undefined, err
			}
			if _, err := invoke(vmi, nextPromise, "then", pc.resolve, pc.reject); err != nil {
				return vm.Undefined, err
			}
		}
	}
}
