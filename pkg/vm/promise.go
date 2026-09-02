package vm

import (
	"fmt"
	"sync"
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
	Handler Value       // Function to call (onFulfilled or onRejected)
	Resolve func(Value) // Resolve the chained promise
	Reject  func(Value) // Reject the chained promise
}

// PromiseObject represents a JavaScript Promise
type PromiseObject struct {
	Object

	// mu guards State, Result, FulfillReactions and RejectReactions - the
	// fields a Promise created via NewPromiseFromExecutor/NewPendingPromise
	// can have settled from a goroutine other than the VM's own execution
	// goroutine (e.g. fetch()'s doFetchRequestWithContext calling
	// vm.ResolvePromise/vm.RejectPromise once its HTTP round-trip
	// completes). Before this lock existed, every read of these fields -
	// most notably the `await` opcode's `switch awaitedPromise.State`
	// (vm.go) and the top-level-await drain loop's `for awaitedPromise.State
	// == PromisePending` - raced unsynchronized against that goroutine's
	// writes; `go test -race` flags this with nothing more than a bare
	// NewPendingPromise() + a goroutine calling ResolvePromise() + a naive
	// polling read, no fetch() or other feature involved. All access to
	// these four fields must go through the methods below (snapshot/
	// trySettle/takeReactions/addReaction), never direct field access.
	//
	// Frame/Function/ThisValue (async-function suspension state) and
	// Properties/prototype are NOT guarded here: they are only ever read or
	// written from the VM's own execution goroutine (resumed exclusively via
	// scheduled microtasks, which always run on that same goroutine), never
	// from an external host goroutine.
	mu               sync.Mutex
	State            PromiseState
	Result           Value // Fulfillment value or rejection reason
	FulfillReactions []PromiseReaction
	RejectReactions  []PromiseReaction

	// For async functions: suspended execution state
	Frame     *SuspendedFrame // Execution frame (nil if not an async function promise)
	Function  Value           // The async function (for resumption)
	ThisValue Value           // The 'this' value when async function was called

	prototype  Value        // Per-instance [[Prototype]] override for subclassing; Undefined = intrinsic
	Properties *PlainObject // User-defined properties on the Promise object (e.g. `class X extends Promise { constructor() { super(...); this.foo = 1; } }`)
}

func (p *PromiseObject) GetPrototype() Value  { return p.prototype }
func (p *PromiseObject) SetPrototype(v Value) { p.prototype = v }

// GetState returns the promise state. Safe to call from any goroutine.
func (p *PromiseObject) GetState() PromiseState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.State
}

// GetResult returns the promise result (value or reason). Safe to call from
// any goroutine.
func (p *PromiseObject) GetResult() Value {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.Result
}

// snapshot returns a mutually consistent (State, Result) pair. Safe to call
// from any goroutine. Prefer this over GetState()+GetResult() separately
// when both are needed together, since two separate locked reads could
// otherwise observe an in-between settlement (a Pending State paired with a
// Result written moments later).
func (p *PromiseObject) snapshot() (PromiseState, Value) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.State, p.Result
}

// trySettle atomically transitions the promise from Pending to state/value
// and reports whether this call performed the transition (false if the
// promise had already settled - matches the spec's [[AlreadyResolved]]
// guard). It does not touch reactions or invoke any callback; the caller is
// responsible for calling triggerPromiseReactions itself on success. Safe to
// call from any goroutine.
func (p *PromiseObject) trySettle(state PromiseState, value Value) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.State != PromisePending {
		return false
	}
	p.State = state
	p.Result = value
	return true
}

// takeReactions atomically reads and clears the reaction list for the given
// disposition (isFulfilled selects FulfillReactions vs RejectReactions).
// Idempotent: a second call with nothing newly added since the first
// returns an empty slice, which is what makes concurrent addReaction +
// trySettle-triggered dispatch race-free without ever double-firing a
// reaction (see the two call sites in triggerPromiseReactions callers).
// Safe to call from any goroutine.
func (p *PromiseObject) takeReactions(isFulfilled bool) []PromiseReaction {
	p.mu.Lock()
	defer p.mu.Unlock()
	if isFulfilled {
		r := p.FulfillReactions
		p.FulfillReactions = nil
		return r
	}
	r := p.RejectReactions
	p.RejectReactions = nil
	return r
}

// addReaction appends a reaction for the given disposition and, in the same
// critical section, reports the promise's current State - so a caller never
// has to separately (and racily) check State before deciding whether to
// also trigger dispatch immediately: whichever of {this append} or {a
// concurrent trySettle+takeReactions pair} the mutex serializes first is
// exactly what determines whether the newly added reaction is picked up by
// that concurrent dispatch or needs to be dispatched by this caller instead
// - and because takeReactions is idempotent, doing both is always safe: at
// most one of them will find the reaction still present. Safe to call from
// any goroutine.
func (p *PromiseObject) addReaction(isFulfilled bool, reaction PromiseReaction) PromiseState {
	p.mu.Lock()
	defer p.mu.Unlock()
	if isFulfilled {
		p.FulfillReactions = append(p.FulfillReactions, reaction)
	} else {
		p.RejectReactions = append(p.RejectReactions, reaction)
	}
	return p.State
}

// addAwaitReactions is addReaction's counterpart for `await`'s own internal
// bookkeeping (vm.go's OpAwait), where - unlike .then()'s chaining, which
// always needs both a fulfill and a reject reaction structurally paired to
// build its returned promise - only ONE of onFulfill/onReject will ever
// actually matter, because a promise's disposition is immutable once
// settled. Determining that disposition and registering only the reaction
// that can still fire happens in one critical section (no separate,
// TOCTOU-prone State check beforehand): if already Fulfilled or Rejected,
// only the matching reaction is appended; if still Pending, both are
// appended (either could still end up firing). This is what keeps a loop
// like `for (;;) { await cachedResolvedPromise; }` from leaking one dead
// reaction (and its captured closure) per iteration onto the opposite,
// permanently-inert reaction list forever - registering unconditionally via
// two addReaction calls, as an earlier version of this fix did, avoided the
// race but not that leak. Returns the disposition observed in the same
// critical section, so the caller knows whether to also trigger dispatch
// immediately. Safe to call from any goroutine.
func (p *PromiseObject) addAwaitReactions(onFulfill, onReject PromiseReaction) PromiseState {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch p.State {
	case PromiseFulfilled:
		p.FulfillReactions = append(p.FulfillReactions, onFulfill)
	case PromiseRejected:
		p.RejectReactions = append(p.RejectReactions, onReject)
	default: // Pending
		p.FulfillReactions = append(p.FulfillReactions, onFulfill)
		p.RejectReactions = append(p.RejectReactions, onReject)
	}
	return p.State
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
			// Per ECMAScript 25.4.3.1 step 10: reject with the executor's own
			// thrown value, not a stringified Go error.
			var reason Value
			if ee, ok := err.(ExceptionError); ok {
				reason = ee.GetExceptionValue()
			} else {
				reason = NewString(err.Error())
			}
			vm.rejectPromise(promise, reason)
			// vm.Call itself now clears vm.unwinding when it hands an
			// exception off as a Go error (#142 - see the comment in
			// executeUserFunctionSafe). It deliberately does NOT clear
			// unwindingCrossedNative, which stays set for re-throw
			// detection, so this ClearUnwindingState() call is still doing
			// real work on that third flag - but for the phantom-exception
			// bug specifically it is now defense in depth rather than the
			// only thing standing between us and it: we're not re-throwing, the
			// executor's exception has been fully absorbed into a rejected
			// promise, a normal (non-erroring) return value from this
			// function, so there is nothing left for the caller to see as
			// still in flight either way.
			vm.ClearUnwindingState()
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

// resolvePromise fulfills a promise with a value. Safe to call from any
// goroutine (see PromiseObject's mu doc comment).
func (vm *VM) resolvePromise(promise *PromiseObject, value Value) {
	// Handle promise resolution with thenable chaining
	if value.Type() == TypePromise {
		otherPromise := value.AsPromise()
		if otherPromise == nil {
			if promise.trySettle(PromiseFulfilled, value) {
				vm.triggerPromiseReactions(promise, true)
			}
			return
		}

		otherState, otherResult := otherPromise.snapshot()
		switch otherState {
		case PromiseFulfilled:
			if promise.trySettle(PromiseFulfilled, otherResult) {
				vm.triggerPromiseReactions(promise, true)
			}
		case PromiseRejected:
			vm.rejectPromise(promise, otherResult)
		default: // Pending: chain to it
			vm.addPromiseReaction(value, true, func(v Value) {
				vm.resolvePromise(promise, v)
			})
			vm.addPromiseReaction(value, false, func(r Value) {
				vm.rejectPromise(promise, r)
			})
		}
		return
	}

	if promise.trySettle(PromiseFulfilled, value) {
		vm.triggerPromiseReactions(promise, true)
	}
}

// rejectPromise rejects a promise with a reason. Safe to call from any
// goroutine (see PromiseObject's mu doc comment).
func (vm *VM) rejectPromise(promise *PromiseObject, reason Value) {
	if promise.trySettle(PromiseRejected, reason) {
		vm.triggerPromiseReactions(promise, false)
	}
}

// triggerPromiseReactions schedules all reactions for a settled promise.
// Must only be called after the promise has actually settled (State is
// Fulfilled/Rejected, checked via trySettle/addReaction's return value by
// every caller) - it reads Result via GetResult() rather than trySettle's
// return so it works equally when called for reactions added after
// settlement (addReaction/PromiseThen's "already settled, trigger
// immediately" paths), not just right after the settling trySettle call.
func (vm *VM) triggerPromiseReactions(promise *PromiseObject, isFulfilled bool) {
	reactions := promise.takeReactions(isFulfilled)
	if len(reactions) == 0 {
		return
	}
	value := promise.GetResult()

	rt := vm.GetAsyncRuntime()
	for _, reaction := range reactions {
		reaction := reaction // Capture for closure

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
				// Reject with the real thrown value, not a stringified Go
				// error - exceptionError's Error() method is a fixed literal
				// ("VM exception"), so a plain NewString(err.Error()) here
				// silently replaced a real thrown Error object (from an
				// ordinary `.then(handler)` where handler throws) with an
				// unrelated string carrying no .message/.stack. Mirrors the
				// identical ExceptionError handling a few lines up in this
				// same file (the executor-rejection path) and the resume
				// paths in vm.go.
				var reason Value
				if ee, ok := err.(ExceptionError); ok {
					reason = ee.GetExceptionValue()
				} else {
					reason = NewString(err.Error())
				}
				reaction.Reject(reason)
				// Mirrors NewPromiseFromExecutor's own call a few lines up in
				// this file. vm.Call now clears vm.unwinding itself when it
				// hands an exception off as a Go error (#142); it leaves
				// unwindingCrossedNative set for re-throw detection, which
				// this call does still clear. This was the actual site that
				// leaked the phantom exception before that fix - a reaction
				// handler's exception is fully absorbed into a rejection
				// right here, a normal (non-erroring) return, so there is
				// nothing left in flight for a caller to see.
				//
				// NOTE for anyone reverting either half: this line and the
				// vm_init.go clear are each independently sufficient for
				// tests/scripts/promise_reaction_throw_no_unwinding_leak.ts,
				// so that test stays green with either one alone. See its
				// header for the measured breakdown.
				vm.ClearUnwindingState()
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

	// addReaction appends and reports the current state in one atomic step -
	// see its doc comment for why checking State separately afterward would
	// be racy against a concurrent settle.
	if isFulfilled {
		if promise.addReaction(true, reaction) == PromiseFulfilled {
			vm.triggerPromiseReactions(promise, true)
		}
	} else {
		if promise.addReaction(false, reaction) == PromiseRejected {
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
					_, _ = vm.Call(resolve, Undefined, []Value{v})
				},
				Reject: func(r Value) {
					_, _ = vm.Call(reject, Undefined, []Value{r})
				},
			}
			// addReaction appends and reports the current state atomically -
			// see its doc comment for why a separate State check afterward
			// would be racy against a concurrent settle.
			if promise.addReaction(true, reaction) == PromiseFulfilled {
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
					_, _ = vm.Call(resolve, Undefined, []Value{v})
				},
				Reject: func(r Value) {
					_, _ = vm.Call(reject, Undefined, []Value{r})
				},
			}
			if promise.addReaction(false, reaction) == PromiseRejected {
				vm.triggerPromiseReactions(promise, false)
			}
		}

		return Undefined, nil
	})

	return vm.NewPromiseFromExecutor(executor)
}

// IterableToArray converts an iterable value to an array
// Supports arrays directly and any object with Symbol.iterator
func (vm *VM) IterableToArray(value Value) (Value, error) {
	// If it's already an array, return it
	if value.Type() == TypeArray {
		return value, nil
	}

	// Try to get Symbol.iterator
	if vm.SymbolIterator.Type() == TypeUndefined {
		return Undefined, fmt.Errorf("value is not iterable")
	}

	// Get value[Symbol.iterator]
	var iteratorMethod Value
	if value.IsObject() {
		// Try to get the Symbol.iterator property using the symbol key
		obj := value.AsPlainObject()
		if obj != nil {
			if method, exists := obj.GetOwnByKey(NewSymbolKey(vm.SymbolIterator)); exists {
				iteratorMethod = method
			}
		}
		// DictObjects don't support symbol keys, so skip them
	}

	// If no iterator method found, it's not iterable
	if iteratorMethod.Type() == TypeUndefined || !iteratorMethod.IsCallable() {
		return Undefined, fmt.Errorf("value is not iterable")
	}

	// Call the iterator method to get the iterator object
	iteratorObj, err := vm.Call(iteratorMethod, value, []Value{})
	if err != nil {
		return Undefined, err
	}

	// Get the next method
	var nextMethod Value
	if iteratorObj.IsObject() {
		obj := iteratorObj.AsPlainObject()
		if obj != nil {
			if next, exists := obj.GetOwn("next"); exists {
				nextMethod = next
			}
		} else if iteratorObj.Type() == TypeDictObject {
			dictObj := iteratorObj.AsDictObject()
			if next, exists := dictObj.GetOwn("next"); exists {
				nextMethod = next
			}
		}
	}

	if !nextMethod.IsCallable() {
		return Undefined, fmt.Errorf("iterator does not have a next method")
	}

	// Collect all values from the iterator
	var elements []Value
	maxIterations := 10000 // Safety limit
	for i := 0; i < maxIterations; i++ {
		// Call next()
		result, err := vm.Call(nextMethod, iteratorObj, []Value{})
		if err != nil {
			return Undefined, err
		}

		// Get result.done
		var done Value = Undefined
		if result.IsObject() {
			obj := result.AsPlainObject()
			if obj != nil {
				if d, exists := obj.GetOwn("done"); exists {
					done = d
				}
			} else if result.Type() == TypeDictObject {
				dictObj := result.AsDictObject()
				if d, exists := dictObj.GetOwn("done"); exists {
					done = d
				}
			}
		}

		// Check if done is truthy (JavaScript semantics)
		// Falsy: false, 0, "", null, undefined
		// Everything else is truthy
		isDone := false
		if done.Type() == TypeBoolean {
			isDone = done.AsBoolean()
		} else if done.IsNumber() {
			isDone = done.ToFloat() != 0
		} else if done.Type() == TypeString {
			isDone = done.ToString() != ""
		} else if done.Type() == TypeNull || done.Type() == TypeUndefined {
			isDone = false
		} else {
			// Objects, arrays, functions etc. are truthy
			isDone = true
		}

		if isDone {
			break
		}

		// Get result.value
		var itemValue Value = Undefined
		if result.IsObject() {
			obj := result.AsPlainObject()
			if obj != nil {
				if v, exists := obj.GetOwn("value"); exists {
					itemValue = v
				}
			} else if result.Type() == TypeDictObject {
				dictObj := result.AsDictObject()
				if v, exists := dictObj.GetOwn("value"); exists {
					itemValue = v
				}
			}
		}

		elements = append(elements, itemValue)
	}

	// Create array from collected elements
	return vm.NewArrayFromSlice(elements), nil
}

// NewArrayFromSlice creates a new array from a slice of values
func (vm *VM) NewArrayFromSlice(elements []Value) Value {
	arr := NewArray()
	arrayObj := arr.AsArray()
	arrayObj.SetElements(elements)
	return arr
}

// NewPendingPromise creates a new promise in pending state
func (vm *VM) NewPendingPromise() Value {
	promise := &PromiseObject{
		State:            PromisePending,
		Result:           Undefined,
		FulfillReactions: []PromiseReaction{},
		RejectReactions:  []PromiseReaction{},
	}
	return Value{typ: TypePromise, obj: promiseToUnsafe(promise)}
}

// ResolvePromise fulfills a promise with a value (exported wrapper)
func (vm *VM) ResolvePromise(promise *PromiseObject, value Value) {
	vm.resolvePromise(promise, value)
}

// RejectPromise rejects a promise with a reason (exported wrapper)
func (vm *VM) RejectPromise(promise *PromiseObject, reason Value) {
	vm.rejectPromise(promise, reason)
}

// AddPromiseReaction adds a reaction to a promise (exported wrapper)
func (vm *VM) AddPromiseReaction(promiseVal Value, isFulfilled bool, callback func(Value)) {
	vm.addPromiseReaction(promiseVal, isFulfilled, callback)
}
