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

// resolvePromise implements the resolve half of CreateResolvingFunctions
// (ES 27.2.1.3.2). It must run on the VM's own execution goroutine: the
// thenable branch below reads and calls JS. Host goroutines go through the
// exported ResolvePromise, which queues the whole thing as a microtask.
func (vm *VM) resolvePromise(promise *PromiseObject, value Value) {
	// Step 1: resolving a promise with itself is a chaining cycle.
	if value.Type() == TypePromise && value.AsPromise() == promise {
		cycleErr := vm.NewTypeError("Chaining cycle detected for promise")
		reason := NewString(cycleErr.Error())
		if ee, ok := cycleErr.(ExceptionError); ok {
			reason = ee.GetExceptionValue()
		}
		vm.ClearUnwindingState()
		vm.rejectPromise(promise, reason)
		return
	}

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

	// Steps 8-12: an object (functions included) with a callable `then` is a
	// thenable and must be assimilated, not used as the fulfillment value.
	// Reading `then` happens here, synchronously, so a plain object with no
	// `then` still fulfills on this tick; only the CALL is deferred to a
	// microtask, which is NewPromiseResolveThenableJob.
	if value.IsObject() || value.IsFunction() {
		then, err := vm.GetProperty(value, "then")
		if err != nil {
			// A throwing `then` getter rejects the promise rather than
			// escaping to whoever called resolve.
			reason := NewString(err.Error())
			if ee, ok := err.(ExceptionError); ok {
				reason = ee.GetExceptionValue()
			}
			vm.ClearUnwindingState()
			vm.rejectPromise(promise, reason)
			return
		}
		if then.IsCallable() {
			vm.scheduleThenableJob(promise, value, then)
			return
		}
	}

	if promise.trySettle(PromiseFulfilled, value) {
		vm.triggerPromiseReactions(promise, true)
	}
}

// scheduleThenableJob is NewPromiseResolveThenableJob (ES 27.2.2.2): it calls
// thenable.then(resolvingFunctions) on a later tick. Whichever of resolve or
// reject runs first wins - that is what trySettle already enforces - and a
// throw from `then` rejects, but only if nothing has settled the promise yet.
func (vm *VM) scheduleThenableJob(promise *PromiseObject, thenable Value, then Value) {
	vm.GetAsyncRuntime().ScheduleMicrotask(func() {
		resolve := NewNativeFunction(1, false, "", func(args []Value) (Value, error) {
			v := Undefined
			if len(args) > 0 {
				v = args[0]
			}
			vm.resolvePromise(promise, v)
			return Undefined, nil
		})
		reject := NewNativeFunction(1, false, "", func(args []Value) (Value, error) {
			r := Undefined
			if len(args) > 0 {
				r = args[0]
			}
			vm.rejectPromise(promise, r)
			return Undefined, nil
		})
		if _, err := vm.Call(then, thenable, []Value{resolve, reject}); err != nil {
			reason := NewString(err.Error())
			if ee, ok := err.(ExceptionError); ok {
				reason = ee.GetExceptionValue()
			}
			vm.ClearUnwindingState()
			vm.rejectPromise(promise, reason)
		}
	})
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

// PromiseThen implements Promise.prototype.then() with the intrinsic %Promise%
// as the result promise's constructor.
func (vm *VM) PromiseThen(thisPromise Value, onFulfilled, onRejected Value) (Value, error) {
	return vm.PromiseThenWith(thisPromise, onFulfilled, onRejected, vm.NewPromiseFromExecutor)
}

// PromiseThenWith is PromiseThen with an explicit result-promise factory.
//
// Per spec Promise.prototype.then does NewPromiseCapability(C) where C is
// SpeciesConstructor(promise, %Promise%), so the chained promise is built by
// the species constructor - that is what makes `subclassPromise.then(...)`
// return an instance of the subclass. newPromise receives the executor the
// reactions are wired through and must return the promise built from it;
// pkg/builtins/promise_init.go passes a Construct(C, executor) closure when C
// is not the intrinsic.
func (vm *VM) PromiseThenWith(thisPromise Value, onFulfilled, onRejected Value, newPromise func(executor Value) (Value, error)) (Value, error) {
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

	return newPromise(executor)
}

// maxIterableToArrayIterations bounds how many elements IterableToArray pulls
// from a custom (Generator or arbitrary Symbol.iterator) iterable before
// giving up. Promise.all/allSettled/any/race all eagerly materialize their
// argument into an array up front, unlike the spec's PerformPromiseAll et al,
// which interleave iteration with per-element resolution - so an iterable
// that never reports done hangs the VM forever converting it, rather than
// getting the chance to fail fast the way a real interleaved implementation
// would (a handful of Test262 tests pair a never-done iterator with a
// same-tick `resolve()`/`.then()` that throws immediately, expecting exactly
// that fast, bounded failure). extractSpreadArguments has no such cap, and
// must not gain one here - `[...infiniteIterator]` genuinely never
// terminates in real engines either, and that's the correct, spec-mandated
// behavior for spread.
const maxIterableToArrayIterations = 10000

// IterableToArray converts an iterable value to an array.
// Supports arrays directly, plus anything else the iterator protocol covers.
//
// This used to hand-roll its own Symbol.iterator lookup and next()/done/value
// walk, guarding every step with `value.IsObject()` before unconditionally
// calling `.AsPlainObject()`. IsObject() is true for the whole object-ish
// span of ValueTypes (TypeObject..TypeProxy), not just TypeObject, so any
// iterable that wasn't a plain object - a generator (Generator[Symbol.iterator]()
// returns itself, a TypeGenerator value, not TypeObject), a Set, a Map, ...
// - made AsPlainObject() panic with "value is not an object" (paserati#293,
// hit via `new AggregateError(someGenerator)`; the same call chain is shared
// by Promise.all/allSettled/any/race).
//
// Types that are inherently finite (String/Arguments/Set/Map - Array is
// handled directly above) delegate to extractSpreadArguments, which already
// implements the iterator protocol correctly for them. Generator and
// everything else fall through to iterateBounded below instead, since those
// can be infinite and this function - unlike `...spread` - needs a cap (see
// maxIterableToArrayIterations).
func (vm *VM) IterableToArray(value Value) (Value, error) {
	// If it's already an array, return it
	if value.Type() == TypeArray {
		return value, nil
	}

	switch value.Type() {
	case TypeString, TypeArguments, TypeSet, TypeMap:
		elements, err := vm.extractSpreadArguments(value)
		if err != nil {
			return Undefined, err
		}
		return vm.NewArrayFromSlice(elements), nil
	default:
		return vm.iterableToArrayBounded(value)
	}
}

// iterableToArrayBounded implements the generic ES6 iterator protocol
// (Symbol.iterator lookup walked across the TypeObject/TypeGenerator/
// TypeAsyncGenerator/TypeDictObject prototype chain, exactly like
// extractSpreadArguments's own default case) capped at
// maxIterableToArrayIterations calls to next().
func (vm *VM) iterableToArrayBounded(value Value) (Value, error) {
	if vm.SymbolIterator.Type() == TypeUndefined {
		return Undefined, vm.NewTypeError(fmt.Sprintf("%s is not iterable", value.TypeName()))
	}

	iteratorMethod := Undefined
	found := false
	iterKey := NewSymbolKey(vm.SymbolIterator)
	current := value

	for current.Type() != TypeNull && current.Type() != TypeUndefined {
		switch current.Type() {
		case TypeObject:
			obj := current.AsPlainObject()
			if g, _, _, _, ok := obj.GetOwnAccessorByKey(iterKey); ok && g.Type() != TypeUndefined {
				res, err := vm.Call(g, value, nil)
				if err != nil {
					return Undefined, err
				}
				iteratorMethod = res
				found = true
			} else if val, ok := obj.GetOwnByKey(iterKey); ok {
				iteratorMethod = val
				found = true
			} else {
				current = obj.prototype
				continue
			}
		case TypeGenerator:
			// Prototype is a plain Value now (not always a *PlainObject; see
			// GeneratorObject's Prototype field, changed for #418) - the
			// loop's own Null/Undefined check at the top handles both "no
			// override" and an explicit Object.setPrototypeOf(gen, null),
			// so no separate nil guard is needed here.
			genObj := current.AsGenerator()
			current = genObj.Prototype
			continue
		case TypeAsyncGenerator:
			genObj := current.AsAsyncGenerator()
			current = genObj.Prototype
			continue
		case TypeDictObject:
			// DictObjects don't support symbol keys, so there's nothing to
			// find on this link - just keep walking its prototype.
			current = current.AsDictObject().prototype
			continue
		default:
			return Undefined, vm.NewTypeError(fmt.Sprintf("%s is not iterable", value.TypeName()))
		}
		break
	}

	if !found || !iteratorMethod.IsCallable() {
		return Undefined, vm.NewTypeError(fmt.Sprintf("%s is not iterable", value.TypeName()))
	}

	iteratorObj, err := vm.Call(iteratorMethod, value, []Value{})
	if err != nil {
		return Undefined, err
	}

	nextMethod, err := vm.GetProperty(iteratorObj, "next")
	if err != nil {
		return Undefined, err
	}
	if !nextMethod.IsCallable() {
		return Undefined, vm.NewTypeError("iterator does not have a next method")
	}

	var elements []Value
	for i := 0; i < maxIterableToArrayIterations; i++ {
		result, err := vm.Call(nextMethod, iteratorObj, []Value{})
		if err != nil {
			return Undefined, err
		}

		done, err := vm.GetProperty(result, "done")
		if err != nil {
			return Undefined, err
		}
		if done.IsTruthy() {
			break
		}

		itemValue, err := vm.GetProperty(result, "value")
		if err != nil {
			return Undefined, err
		}
		elements = append(elements, itemValue)
	}

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

// ResolvePromise fulfills a promise with a value. This is the entry point for
// host goroutines (fetch, ReadableStream, timers), which must never execute JS
// themselves - so when the value might be a thenable, the whole resolution is
// queued as a microtask and runs on the VM's goroutine instead. Queuing still
// happens-before the caller returns, so the runtime sees pending work and stays
// awake (#238). Everything else settles synchronously, as it always has.
func (vm *VM) ResolvePromise(promise *PromiseObject, value Value) {
	if vm.hasPotentialThen(value) {
		vm.GetAsyncRuntime().ScheduleMicrotask(func() {
			vm.resolvePromise(promise, value)
		})
		return
	}
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

// hasPotentialThen reports whether v could be a thenable, WITHOUT running any
// JS: it looks for a `then` slot - data property or accessor - on v's own
// storage and along its [[Prototype]] chain, but never invokes a getter.
//
// This exists for ResolvePromise, the entry point host goroutines use (fetch's
// HTTP goroutine, ReadableStream's pump). Those must not execute JS, so they
// cannot do the spec's Get(resolution, "then") inline; but they also must not
// unconditionally defer, because a settle that happens after the caller's
// EndExternalOp loses the happen-before that keeps the runtime awake (#238).
// A conservative "no `then` anywhere" answer lets the overwhelmingly common
// case - resolving with a Response, an iterator result, an array - settle
// synchronously exactly as before.
func (vm *VM) hasPotentialThen(v Value) bool {
	if !v.IsObject() && !v.IsFunction() {
		return false
	}
	current := v
	for i := 0; i < 100; i++ {
		if props := OwnPropertiesTable(current); props != nil {
			if _, _, _, _, isAccessor := props.GetOwnAccessor("then"); isAccessor {
				return true
			}
			if _, ok := props.GetOwn("then"); ok {
				return true
			}
		}
		switch current.Type() {
		case TypeObject:
			po := current.AsPlainObject()
			if _, _, _, _, isAccessor := po.GetOwnAccessor("then"); isAccessor {
				return true
			}
			if _, ok := po.GetOwn("then"); ok {
				return true
			}
		case TypeDictObject:
			if _, ok := current.AsDictObject().GetOwn("then"); ok {
				return true
			}
		case TypeProxy:
			// A proxy's get trap is arbitrary JS; assume it may produce one.
			return true
		}
		next := vm.prototypeOf(current)
		if next.Type() == TypeNull || next.Type() == TypeUndefined || next.Equals(current) {
			return false
		}
		current = next
	}
	return false
}
