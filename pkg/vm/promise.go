package vm

import (
	"fmt"
	"unsafe"
)

// Helper to convert PromiseObject pointer to unsafe.Pointer
func promiseToUnsafe(p *PromiseObject) unsafe.Pointer {
	return unsafe.Pointer(p)
}

// PromiseState represents the state of a Promise
type PromiseState int

const (
	PromisePending PromiseState = iota
	PromiseFulfilled
	PromiseRejected
)

// PromiseReaction represents a callback registered via .then()
type PromiseReaction struct {
	Handler Value          // Function to call (onFulfilled or onRejected)
	Resolve func(Value)    // Resolve the chained promise
	Reject  func(Value)    // Reject the chained promise
}

// PromiseObject represents a JavaScript Promise
type PromiseObject struct {
	Object
	State            PromiseState
	Result           Value // Fulfillment value or rejection reason
	FulfillReactions []PromiseReaction
	RejectReactions  []PromiseReaction
}

// GetState returns the promise state
func (p *PromiseObject) GetState() PromiseState {
	return p.State
}

// GetResult returns the promise result (value or reason)
func (p *PromiseObject) GetResult() Value {
	return p.Result
}

// NewPromiseFromExecutor creates a new Promise with an executor function
// executor receives (resolve, reject) functions
func (vm *VM) NewPromiseFromExecutor(executor Value) (Value, error) {
	promise := &PromiseObject{
		State:            PromisePending,
		Result:           Undefined,
		FulfillReactions: []PromiseReaction{},
		RejectReactions:  []PromiseReaction{},
	}

	// Set up prototype chain later when PromisePrototype is available
	promiseVal := Value{typ: TypePromise, obj: promiseToUnsafe(promise)}

	// Create resolve function
	resolve := NewNativeFunction(1, false, "resolve", func(args []Value) (Value, error) {
		value := Undefined
		if len(args) > 0 {
			value = args[0]
		}
		vm.resolvePromise(promise, value)
		return Undefined, nil
	})

	// Create reject function
	reject := NewNativeFunction(1, false, "reject", func(args []Value) (Value, error) {
		reason := Undefined
		if len(args) > 0 {
			reason = args[0]
		}
		vm.rejectPromise(promise, reason)
		return Undefined, nil
	})

	// Call executor(resolve, reject)
	if executor.IsCallable() {
		_, err := vm.Call(executor, Undefined, []Value{resolve, reject})
		if err != nil {
			vm.rejectPromise(promise, NewString(err.Error()))
		}
	}

	return promiseVal, nil
}

// NewResolvedPromise creates a promise that is already fulfilled
func (vm *VM) NewResolvedPromise(value Value) Value {
	promise := &PromiseObject{
		State:            PromiseFulfilled,
		Result:           value,
		FulfillReactions: []PromiseReaction{},
		RejectReactions:  []PromiseReaction{},
	}

	return Value{typ: TypePromise, obj: promiseToUnsafe(promise)}
}

// NewRejectedPromise creates a promise that is already rejected
func (vm *VM) NewRejectedPromise(reason Value) Value {
	promise := &PromiseObject{
		State:            PromiseRejected,
		Result:           reason,
		FulfillReactions: []PromiseReaction{},
		RejectReactions:  []PromiseReaction{},
	}

	return Value{typ: TypePromise, obj: promiseToUnsafe(promise)}
}

// resolvePromise fulfills a promise with a value
func (vm *VM) resolvePromise(promise *PromiseObject, value Value) {
	if promise.State != PromisePending {
		return // Already settled
	}

	// Handle promise resolution with thenable chaining
	if value.Type() == TypePromise {
		otherPromise := value.AsPromise()
		if otherPromise == nil {
			promise.State = PromiseFulfilled
			promise.Result = value
			vm.triggerPromiseReactions(promise, true)
			return
		}

		if otherPromise.State == PromiseFulfilled {
			value = otherPromise.Result
		} else if otherPromise.State == PromiseRejected {
			vm.rejectPromise(promise, otherPromise.Result)
			return
		} else {
			// Chain to pending promise
			vm.addPromiseReaction(value, true, func(v Value) {
				vm.resolvePromise(promise, v)
			})
			vm.addPromiseReaction(value, false, func(r Value) {
				vm.rejectPromise(promise, r)
			})
			return
		}
	}

	promise.State = PromiseFulfilled
	promise.Result = value
	vm.triggerPromiseReactions(promise, true)
}

// rejectPromise rejects a promise with a reason
func (vm *VM) rejectPromise(promise *PromiseObject, reason Value) {
	if promise.State != PromisePending {
		return // Already settled
	}

	promise.State = PromiseRejected
	promise.Result = reason
	vm.triggerPromiseReactions(promise, false)
}

// triggerPromiseReactions schedules all reactions for a settled promise
func (vm *VM) triggerPromiseReactions(promise *PromiseObject, isFulfilled bool) {
	var reactions []PromiseReaction
	if isFulfilled {
		reactions = promise.FulfillReactions
		promise.FulfillReactions = nil
	} else {
		reactions = promise.RejectReactions
		promise.RejectReactions = nil
	}

	rt := vm.GetAsyncRuntime()
	for _, reaction := range reactions {
		reaction := reaction // Capture for closure
		value := promise.Result

		rt.ScheduleMicrotask(func() {
			if reaction.Handler.Type() == 0 || reaction.Handler.Type() == TypeUndefined {
				// No handler - pass through
				if isFulfilled {
					reaction.Resolve(value)
				} else {
					reaction.Reject(value)
				}
				return
			}

			// Call handler
			result, err := vm.Call(reaction.Handler, Undefined, []Value{value})
			if err != nil {
				reaction.Reject(NewString(err.Error()))
			} else {
				reaction.Resolve(result)
			}
		})
	}
}

// addPromiseReaction adds a reaction to a promise
func (vm *VM) addPromiseReaction(promiseVal Value, isFulfilled bool, callback func(Value)) {
	promise := promiseVal.AsPromise()
	if promise == nil {
		return
	}

	reaction := PromiseReaction{
		Handler: Undefined,
		Resolve: callback,
		Reject:  callback,
	}

	if isFulfilled {
		promise.FulfillReactions = append(promise.FulfillReactions, reaction)
		// If already fulfilled, trigger immediately
		if promise.State == PromiseFulfilled {
			vm.triggerPromiseReactions(promise, true)
		}
	} else {
		promise.RejectReactions = append(promise.RejectReactions, reaction)
		// If already rejected, trigger immediately
		if promise.State == PromiseRejected {
			vm.triggerPromiseReactions(promise, false)
		}
	}
}

// PromiseThen implements Promise.prototype.then()
func (vm *VM) PromiseThen(thisPromise Value, onFulfilled, onRejected Value) (Value, error) {
	promise := thisPromise.AsPromise()
	if promise == nil {
		return Undefined, fmt.Errorf("TypeError: Promise.prototype.then called on non-Promise")
	}

	// Create executor for chained promise
	executor := NewNativeFunction(2, false, "executor", func(execArgs []Value) (Value, error) {
		resolve := execArgs[0]
		reject := execArgs[1]

		// Handle fulfillment
		if onFulfilled.IsCallable() || onFulfilled.Type() == TypeUndefined {
			handler := onFulfilled
			if !handler.IsCallable() {
				handler = Undefined
			}

			reaction := PromiseReaction{
				Handler: handler,
				Resolve: func(v Value) {
					vm.Call(resolve, Undefined, []Value{v})
				},
				Reject: func(r Value) {
					vm.Call(reject, Undefined, []Value{r})
				},
			}
			promise.FulfillReactions = append(promise.FulfillReactions, reaction)

			// If already fulfilled, trigger immediately
			if promise.State == PromiseFulfilled {
				vm.triggerPromiseReactions(promise, true)
			}
		}

		// Handle rejection
		if onRejected.IsCallable() || onRejected.Type() == TypeUndefined {
			handler := onRejected
			if !handler.IsCallable() {
				handler = Undefined
			}

			reaction := PromiseReaction{
				Handler: handler,
				Resolve: func(v Value) {
					vm.Call(resolve, Undefined, []Value{v})
				},
				Reject: func(r Value) {
					vm.Call(reject, Undefined, []Value{r})
				},
			}
			promise.RejectReactions = append(promise.RejectReactions, reaction)

			// If already rejected, trigger immediately
			if promise.State == PromiseRejected {
				vm.triggerPromiseReactions(promise, false)
			}
		}

		return Undefined, nil
	})

	return vm.NewPromiseFromExecutor(executor)
}
