package vm

// CreateAsyncFromSyncIterator (ECMAScript 27.1.6). The wrapper is never
// exposed to user code - only for-await and yield* see it - so its
// next/return/throw are per-instance closures over the sync iterator record
// rather than methods on a shared %AsyncFromSyncIteratorPrototype%.

// CreateAsyncFromSyncIterator wraps syncIter, reading its next method once.
func (vm *VM) CreateAsyncFromSyncIterator(syncIter Value) (Value, error) {
	nextMethod, err := vm.GetProperty(syncIter, "next")
	if err != nil {
		return Undefined, err
	}
	obj := NewObject(vm.AsyncIteratorPrototype).AsPlainObject()

	// argsOrNone passes the argument on only when one was given.
	argsOrNone := func(args []Value) []Value {
		if len(args) > 0 {
			return args[:1]
		}
		return nil
	}

	obj.SetOwnNonEnumerable("next", NewNativeFunction(1, false, "next", func(args []Value) (Value, error) {
		promise := vm.newIntrinsicPromise()
		if !nextMethod.IsCallable() {
			vm.rejectPromise(promise, vm.typeErrorValue("The iterator's next method is not a function"))
			return promiseValue(promise), nil
		}
		result, err := vm.Call(nextMethod, syncIter, argsOrNone(args))
		if err != nil {
			vm.asyncFromSyncReject(promise, err)
			return promiseValue(promise), nil
		}
		if !isObjectLike(result) {
			vm.rejectPromise(promise, vm.typeErrorValue("Iterator result "+result.ToString()+" is not an object"))
			return promiseValue(promise), nil
		}
		vm.asyncFromSyncContinuation(result, promise, syncIter, true)
		return promiseValue(promise), nil
	}))

	obj.SetOwnNonEnumerable("return", NewNativeFunction(1, false, "return", func(args []Value) (Value, error) {
		promise := vm.newIntrinsicPromise()
		returnMethod, err := vm.getMethod(syncIter, "return")
		if err != nil {
			vm.asyncFromSyncReject(promise, err)
			return promiseValue(promise), nil
		}
		if returnMethod.Type() == TypeUndefined {
			value := Undefined
			if len(args) > 0 {
				value = args[0]
			}
			vm.resolvePromise(promise, vm.newIterResult(value, true))
			return promiseValue(promise), nil
		}
		result, err := vm.Call(returnMethod, syncIter, argsOrNone(args))
		if err != nil {
			vm.asyncFromSyncReject(promise, err)
			return promiseValue(promise), nil
		}
		if !isObjectLike(result) {
			vm.rejectPromise(promise, vm.typeErrorValue("Iterator result "+result.ToString()+" is not an object"))
			return promiseValue(promise), nil
		}
		vm.asyncFromSyncContinuation(result, promise, syncIter, false)
		return promiseValue(promise), nil
	}))

	obj.SetOwnNonEnumerable("throw", NewNativeFunction(1, false, "throw", func(args []Value) (Value, error) {
		promise := vm.newIntrinsicPromise()
		throwMethod, err := vm.getMethod(syncIter, "throw")
		if err != nil {
			vm.asyncFromSyncReject(promise, err)
			return promiseValue(promise), nil
		}
		if throwMethod.Type() == TypeUndefined {
			// Close the sync iterator so it can clean up, then reject for
			// the protocol violation.
			if err := vm.iteratorCloseNormal(syncIter); err != nil {
				vm.asyncFromSyncReject(promise, err)
				return promiseValue(promise), nil
			}
			vm.rejectPromise(promise, vm.typeErrorValue("The iterator does not provide a 'throw' method"))
			return promiseValue(promise), nil
		}
		result, err := vm.Call(throwMethod, syncIter, argsOrNone(args))
		if err != nil {
			vm.asyncFromSyncReject(promise, err)
			return promiseValue(promise), nil
		}
		if !isObjectLike(result) {
			vm.rejectPromise(promise, vm.typeErrorValue("Iterator result "+result.ToString()+" is not an object"))
			return promiseValue(promise), nil
		}
		vm.asyncFromSyncContinuation(result, promise, syncIter, true)
		return promiseValue(promise), nil
	}))

	return NewValueFromPlainObject(obj), nil
}

func (vm *VM) asyncFromSyncReject(promise *PromiseObject, err error) {
	vm.ClearUnwindingState()
	vm.rejectPromise(promise, vm.ExceptionValueFromError(err))
}

// iteratorCloseNormal is IteratorClose(iterator, NormalCompletion).
func (vm *VM) iteratorCloseNormal(iter Value) error {
	returnMethod, err := vm.getMethod(iter, "return")
	if err != nil {
		return err
	}
	if returnMethod.Type() == TypeUndefined {
		return nil
	}
	inner, err := vm.Call(returnMethod, iter, nil)
	if err != nil {
		return err
	}
	if !isObjectLike(inner) {
		return vm.NewTypeError("Iterator result " + inner.ToString() + " is not an object")
	}
	return nil
}

// iteratorCloseThrow is IteratorClose(iterator, ThrowCompletion): the
// original error wins over anything return() does.
func (vm *VM) iteratorCloseThrow(iter Value) {
	returnMethod, err := vm.getMethod(iter, "return")
	if err != nil {
		vm.ClearUnwindingState()
		return
	}
	if returnMethod.Type() == TypeUndefined {
		return
	}
	if _, err := vm.Call(returnMethod, iter, nil); err != nil {
		vm.ClearUnwindingState()
	}
}

// AsyncIteratorDispose is %AsyncIteratorPrototype%[@@asyncDispose] (27.1.3.1):
// call this.return(undefined) and resolve with undefined once it settles.
func (vm *VM) AsyncIteratorDispose(this Value) Value {
	promise := vm.newIntrinsicPromise()
	returnMethod, err := vm.getMethod(this, "return")
	if err != nil {
		vm.asyncFromSyncReject(promise, err)
		return promiseValue(promise)
	}
	if returnMethod.Type() == TypeUndefined {
		vm.resolvePromise(promise, Undefined)
		return promiseValue(promise)
	}
	result, err := vm.Call(returnMethod, this, []Value{Undefined})
	if err != nil {
		vm.asyncFromSyncReject(promise, err)
		return promiseValue(promise)
	}
	wrapper, err := vm.awaitPromiseResolve(result)
	if err != nil {
		vm.asyncFromSyncReject(promise, err)
		return promiseValue(promise)
	}
	vm.thenGo(wrapper, func(Value) {
		vm.resolvePromise(promise, Undefined)
	}, func(r Value) {
		vm.rejectPromise(promise, r)
	})
	return promiseValue(promise)
}

// asyncFromSyncContinuation is AsyncFromSyncIteratorContinuation.
func (vm *VM) asyncFromSyncContinuation(result Value, promise *PromiseObject, syncIter Value, closeOnRejection bool) {
	doneVal, err := vm.GetProperty(result, "done")
	if err != nil {
		vm.asyncFromSyncReject(promise, err)
		return
	}
	done := doneVal.IsTruthy()
	value, err := vm.GetProperty(result, "value")
	if err != nil {
		vm.asyncFromSyncReject(promise, err)
		return
	}
	wrapper, err := vm.awaitPromiseResolve(value)
	if err != nil {
		if !done && closeOnRejection {
			vm.ClearUnwindingState()
			vm.iteratorCloseThrow(syncIter)
		}
		vm.asyncFromSyncReject(promise, err)
		return
	}
	vm.thenGo(wrapper, func(v Value) {
		vm.resolvePromise(promise, vm.newIterResult(v, done))
	}, func(r Value) {
		if !done && closeOnRejection {
			vm.iteratorCloseThrow(syncIter)
		}
		vm.rejectPromise(promise, r)
	})
}
