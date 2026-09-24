package builtins

import (
	"strconv"

	"github.com/nooga/paserati/pkg/vm"
)

// arrayFromAsync is Array.fromAsync(asyncItems, mapfn, thisArg) (proposal
// spec, 2.1.1.1). The spec runs its body as an async function; here that body
// is written in continuation-passing style over vm.AwaitThen, so each Await
// resumes in a later job exactly as the spec's would.
func arrayFromAsync(vmi *vm.VM, c, asyncItems, mapfn, thisArg vm.Value) vm.Value {
	promise := vmi.NewPendingPromise()
	resolveFn, rejectFn := vmi.CreateResolvingFunctions(promise.AsPromise())
	resolve := func(v vm.Value) { _, _ = vmi.Call(resolveFn, vm.Undefined, []vm.Value{v}) }
	rejectValue := func(r vm.Value) { _, _ = vmi.Call(rejectFn, vm.Undefined, []vm.Value{r}) }
	fail := func(err error) {
		vmi.ClearUnwindingState()
		rejectValue(vmi.ExceptionValueFromError(err))
	}

	mapping := mapfn.Type() != vm.TypeUndefined
	if mapping && !mapfn.IsCallable() {
		fail(vmi.NewTypeError("Array.fromAsync: mapper is not a function"))
		return promise
	}
	if asyncItems.Type() == vm.TypeUndefined || asyncItems.Type() == vm.TypeNull {
		fail(vmi.NewTypeError("Cannot convert undefined or null to object"))
		return promise
	}

	usingAsync, err := getSymbolMethod(vmi, asyncItems, SymbolAsyncIterator)
	if err != nil {
		fail(err)
		return promise
	}
	usingSync := vm.Undefined
	if usingAsync.Type() == vm.TypeUndefined {
		if usingSync, err = getSymbolMethod(vmi, asyncItems, SymbolIterator); err != nil {
			fail(err)
			return promise
		}
	}

	// mapValue applies mapfn (awaiting its result) and hands the value on;
	// onAbrupt receives a throw from either step.
	mapValue := func(v vm.Value, k int, then func(vm.Value), onAbrupt func(vm.Value)) {
		if !mapping {
			then(v)
			return
		}
		mv, err := vmi.Call(mapfn, thisArg, []vm.Value{v, vm.NumberValue(float64(k))})
		if err != nil {
			vmi.ClearUnwindingState()
			onAbrupt(vmi.ExceptionValueFromError(err))
			return
		}
		vmi.AwaitThen(mv, then, onAbrupt)
	}

	if usingAsync.Type() != vm.TypeUndefined || usingSync.Type() != vm.TypeUndefined {
		var iter vm.Value
		if usingAsync.Type() != vm.TypeUndefined {
			iter, err = vmi.Call(usingAsync, asyncItems, nil)
		} else if iter, err = vmi.Call(usingSync, asyncItems, nil); err == nil {
			if !isObjectValue(iter) {
				err = vmi.NewTypeError("Result of the Symbol.iterator method is not an object")
			} else {
				iter, err = vmi.CreateAsyncFromSyncIterator(iter)
			}
		}
		if err == nil && !isObjectValue(iter) {
			err = vmi.NewTypeError("Result of the Symbol.asyncIterator method is not an object")
		}
		if err != nil {
			fail(err)
			return promise
		}
		next, err := vmi.GetProperty(iter, "next")
		if err != nil {
			fail(err)
			return promise
		}
		var a vm.Value
		if vmi.IsConstructor(c) {
			if a, err = vmi.Construct(c, nil); err != nil {
				fail(err)
				return promise
			}
		} else {
			a = vm.NewArray()
		}

		// closeWith is AsyncIteratorClose(iteratorRecord, throw completion):
		// return() is called and its result awaited, but the original
		// error is what rejects.
		closeWith := func(reason vm.Value) {
			ret, err := vmi.GetProperty(iter, "return")
			if err != nil || ret.Type() == vm.TypeUndefined || ret.Type() == vm.TypeNull || !ret.IsCallable() {
				vmi.ClearUnwindingState()
				rejectValue(reason)
				return
			}
			res, err := vmi.Call(ret, iter, nil)
			if err != nil {
				vmi.ClearUnwindingState()
				rejectValue(reason)
				return
			}
			vmi.AwaitThen(res, func(vm.Value) { rejectValue(reason) }, func(vm.Value) { rejectValue(reason) })
		}

		var step func(k int)
		step = func(k int) {
			if !next.IsCallable() {
				fail(vmi.NewTypeError("iterator.next is not a function"))
				return
			}
			res, err := vmi.Call(next, iter, nil)
			if err != nil {
				fail(err)
				return
			}
			vmi.AwaitThen(res, func(r vm.Value) {
				if !isObjectValue(r) {
					fail(vmi.NewTypeError("Iterator result " + r.ToString() + " is not an object"))
					return
				}
				done, err := vmi.GetProperty(r, "done")
				if err != nil {
					fail(err)
					return
				}
				if done.IsTruthy() {
					if err := setLengthOrThrow(vmi, a, k); err != nil {
						fail(err)
						return
					}
					resolve(a)
					return
				}
				v, err := vmi.GetProperty(r, "value")
				if err != nil {
					fail(err)
					return
				}
				mapValue(v, k, func(mv vm.Value) {
					if err := createDataPropertyOrThrow(vmi, a, k, mv); err != nil {
						vmi.ClearUnwindingState()
						closeWith(vmi.ExceptionValueFromError(err))
						return
					}
					step(k + 1)
				}, closeWith)
			}, rejectValue)
		}
		step(0)
		return promise
	}

	// Array-like: read length, then await each element in turn.
	length, err := arrayLikeLength(vmi, asyncItems)
	if err != nil {
		fail(err)
		return promise
	}
	var a vm.Value
	if vmi.IsConstructor(c) {
		if a, err = vmi.Construct(c, []vm.Value{vm.NumberValue(float64(length))}); err != nil {
			fail(err)
			return promise
		}
	} else {
		a = vm.NewArrayWithLength(length)
	}
	var step func(k int)
	step = func(k int) {
		if k >= length {
			if err := setLengthOrThrow(vmi, a, length); err != nil {
				fail(err)
				return
			}
			resolve(a)
			return
		}
		kValue, err := vmi.GetProperty(asyncItems, strconv.Itoa(k))
		if err != nil {
			fail(err)
			return
		}
		vmi.AwaitThen(kValue, func(v vm.Value) {
			mapValue(v, k, func(mv vm.Value) {
				if err := createDataPropertyOrThrow(vmi, a, k, mv); err != nil {
					fail(err)
					return
				}
				step(k + 1)
			}, rejectValue)
		}, rejectValue)
	}
	step(0)
	return promise
}

// createDataPropertyOrThrow is CreateDataPropertyOrThrow(a, ToString(k), v),
// going through the generic [[DefineOwnProperty]] so it works on whatever
// object a subclass constructor returned.
func createDataPropertyOrThrow(vmi *vm.VM, a vm.Value, k int, v vm.Value) error {
	desc := vm.NewObject(vmi.ObjectPrototype).AsPlainObject()
	desc.SetOwn("value", v)
	desc.SetOwn("writable", vm.BooleanValue(true))
	desc.SetOwn("enumerable", vm.BooleanValue(true))
	desc.SetOwn("configurable", vm.BooleanValue(true))
	_, err := objectDefinePropertyWithVM(vmi, []vm.Value{a, vm.NewString(strconv.Itoa(k)), vm.NewValueFromPlainObject(desc)})
	return err
}

// setLengthOrThrow is Set(a, "length", n, true).
func setLengthOrThrow(vmi *vm.VM, a vm.Value, n int) error {
	if a.Type() == vm.TypeArray {
		return vmi.SetProperty(a, "length", vm.NumberValue(float64(n)))
	}
	set := reflectOrdinarySet
	if a.Type() == vm.TypeProxy {
		set = reflectProxySet
	}
	ok, err := set(vmi, a, "length", vm.NumberValue(float64(n)), a)
	if err != nil {
		return err
	}
	if !ok {
		return vmi.NewTypeError("Cannot assign to read only property 'length'")
	}
	return nil
}
