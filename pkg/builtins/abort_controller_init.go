package builtins

import (
	"sync"
	"time"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// Priority constant for AbortController
const PriorityAbortController = 195 // Before fetch

type AbortControllerInitializer struct{}

func (a *AbortControllerInitializer) Name() string {
	return "AbortController"
}

func (a *AbortControllerInitializer) Priority() int {
	return PriorityAbortController
}

func (a *AbortControllerInitializer) InitTypes(ctx *TypeContext) error {
	// AbortSignal type
	abortSignalType := types.NewObjectType().
		WithProperty("aborted", types.Boolean).
		WithProperty("reason", types.Any).
		WithProperty("onabort", types.Any).
		WithProperty("throwIfAborted", types.NewSimpleFunction([]types.Type{}, types.Undefined)).
		WithProperty("addEventListener", types.NewOptionalFunction([]types.Type{types.String, types.Any, types.Any}, types.Undefined, []bool{false, false, true})).
		WithProperty("removeEventListener", types.NewOptionalFunction([]types.Type{types.String, types.Any, types.Any}, types.Undefined, []bool{false, false, true}))

	// AbortSignal static methods
	abortSignalConstructorType := types.NewObjectType().
		WithProperty("abort", types.NewOptionalFunction([]types.Type{types.Any}, abortSignalType, []bool{true})).
		WithProperty("timeout", types.NewSimpleFunction([]types.Type{types.Number}, abortSignalType)).
		WithProperty("any", types.NewSimpleFunction([]types.Type{types.Any}, abortSignalType)). // signals array
		WithProperty("prototype", abortSignalType)

	if err := ctx.DefineGlobal("AbortSignal", abortSignalConstructorType); err != nil {
		return err
	}

	// AbortController type
	abortControllerType := types.NewObjectType().
		WithProperty("signal", abortSignalType).
		WithProperty("abort", types.NewOptionalFunction([]types.Type{types.Any}, types.Undefined, []bool{true}))

	// AbortController constructor
	abortControllerConstructorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, abortControllerType).
		WithProperty("prototype", abortControllerType)

	return ctx.DefineGlobal("AbortController", abortControllerConstructorType)
}

func (a *AbortControllerInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create AbortSignal.prototype
	signalProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// AbortSignal is not directly constructible, but we need the static methods
	signalConstructor := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// AbortSignal.abort(reason?) - creates an already-aborted signal
	signalConstructor.SetOwnNonEnumerable("abort", vm.NewNativeFunction(1, false, "abort", func(args []vm.Value) (vm.Value, error) {
		var reason vm.Value
		if len(args) > 0 && !args[0].IsUndefined() {
			reason = args[0]
		} else {
			// Default reason is a real Error named "AbortError" (there's no
			// DOMException in this runtime - see newAbortErrorValue in
			// fetch_init.go, which this reuses so fetch()'s rejection and a
			// bare AbortSignal.abort() produce the same shape of reason).
			reason = newAbortErrorValue(vmInstance, "signal is aborted without reason")
		}
		signal := &AbortSignal{
			aborted: true,
			reason:  reason,
			onabort: vm.Null,
		}
		return createAbortSignalObject(vmInstance, signal, signalProto), nil
	}))

	// AbortSignal.timeout(ms) - creates a signal that aborts after timeout.
	//
	// Scheduled via ScheduleUnrefTimer (#374), not ScheduleTimer: the VM's
	// drain loop blocks on AsyncRuntime.HasPendingTimers()/
	// WaitForIdleProgress() (see pkg/vm/vm.go), so a plain timer here would
	// keep every script alive for the full timeout duration even after
	// everything else finished - turning `AbortSignal.timeout(30000)` into
	// an accidental 30s hang. An unref'd timer doesn't by itself justify
	// waiting, but still fires normally once due if something else (e.g.
	// the fetch() it's guarding) is keeping the loop alive anyway - which
	// is the only situation in which anything is listening for it.
	signalConstructor.SetOwnNonEnumerable("timeout", vm.NewNativeFunction(1, false, "timeout", func(args []vm.Value) (vm.Value, error) {
		ms := 0.0
		if len(args) > 0 {
			ms = args[0].ToFloat()
		}
		if ms < 0 {
			ms = 0
		}

		signal := &AbortSignal{
			aborted: false,
			reason:  vm.Undefined,
			onabort: vm.Null,
		}
		signalValue := createAbortSignalObject(vmInstance, signal, signalProto)

		rt := vmInstance.GetAsyncRuntime()
		rt.ScheduleUnrefTimer(time.Duration(ms)*time.Millisecond, func() {
			reason := newErrorValueWithPrototype(vmInstance.ErrorPrototype, "TimeoutError", "signal timed out")
			triggerAbort(vmInstance, signal, reason)
		})

		return signalValue, nil
	}))

	// AbortSignal.any(signals) - creates a signal that aborts when any input
	// signal aborts. Per spec this must also react to a source aborting
	// *later*, not just one that's already aborted at call time: for every
	// source that isn't aborted yet, we register a listener (through the
	// source's own addEventListener, exactly as user code would) that
	// propagates the abort to the composite signal when it fires.
	signalConstructor.SetOwnNonEnumerable("any", vm.NewNativeFunction(1, false, "any", func(args []vm.Value) (vm.Value, error) {
		signal := &AbortSignal{
			aborted: false,
			reason:  vm.Undefined,
			onabort: vm.Null,
		}
		signalValue := createAbortSignalObject(vmInstance, signal, signalProto)

		if len(args) > 0 && args[0].Type() == vm.TypeArray {
			arr := args[0].AsArray()
			for i := 0; i < arr.Length(); i++ {
				elem := arr.Get(i)
				if elem.Type() != vm.TypeObject {
					continue
				}
				srcObj := elem.AsPlainObject()

				if aborted, exists := srcObj.GetOwn("aborted"); exists && aborted.IsBoolean() && aborted.AsBoolean() {
					if !signal.aborted {
						reason, _ := srcObj.GetOwn("reason")
						triggerAbort(vmInstance, signal, reason)
					}
					continue
				}

				addListenerFn, exists := srcObj.GetOwn("addEventListener")
				if !exists || !addListenerFn.IsCallable() {
					continue
				}
				source := srcObj // per-iteration capture for the closure below
				propagate := vm.NewNativeFunction(1, false, "", func(_ []vm.Value) (vm.Value, error) {
					reason, _ := source.GetOwn("reason")
					triggerAbort(vmInstance, signal, reason)
					return vm.Undefined, nil
				})
				_, _ = vmInstance.Call(addListenerFn, elem, []vm.Value{vm.NewString("abort"), propagate})
			}
		}

		return signalValue, nil
	}))

	signalConstructor.DefineFixedProperty("prototype", vm.NewValueFromPlainObject(signalProto))
	signalProto.SetOwnNonEnumerable("constructor", vm.NewValueFromPlainObject(signalConstructor))

	if err := ctx.DefineGlobal("AbortSignal", vm.NewValueFromPlainObject(signalConstructor)); err != nil {
		return err
	}

	// Create AbortController.prototype
	controllerProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// AbortController constructor
	controllerConstructorFn := func(args []vm.Value) (vm.Value, error) {
		signal := &AbortSignal{
			aborted: false,
			reason:  vm.Undefined,
			onabort: vm.Null,
		}
		controller := &AbortController{
			signal: signal,
		}
		return createAbortControllerObject(vmInstance, controller, controllerProto, signalProto), nil
	}

	controllerConstructor := vm.NewConstructorWithProps(0, false, "AbortController", controllerConstructorFn)
	if controllerConstructor.Type() == vm.TypeNativeFunctionWithProps {
		ctorProps := controllerConstructor.AsNativeFunctionWithProps()
		ctorProps.Properties.DefineFixedProperty("prototype", vm.NewValueFromPlainObject(controllerProto))
	}

	controllerProto.SetOwnNonEnumerable("constructor", controllerConstructor)

	return ctx.DefineGlobal("AbortController", controllerConstructor)
}

// abortListener is one addEventListener("abort", ...) registration.
type abortListener struct {
	callback vm.Value
	once     bool
	// removed guards against a listener firing when it was removed by an
	// *earlier* listener during the same dispatch - dispatchAbort snapshots
	// signal.listeners before calling out to JS, and a removeEventListener
	// call from within one callback mutates this shared struct, which the
	// snapshot's remaining iterations must see (per DOM event dispatch).
	removed bool
}

// AbortSignal represents the signal object
type AbortSignal struct {
	mu        sync.Mutex
	aborted   bool
	reason    vm.Value
	listeners []*abortListener
	onabort   vm.Value // the onabort IDL attribute; vm.Null when unset
	// jsObj is the exposed JS object for this signal. Its "aborted"/"reason"
	// own properties are kept in sync with the fields above on every
	// triggerAbort call (rather than exposed as accessors) because fetch()
	// polls them with a raw GetOwn from a background goroutine - GetOwn
	// never invokes getters, and calling into the VM from that goroutine to
	// resolve one would not be goroutine-safe (see vm.Call's doc comments
	// in fetch_init.go). Keeping them plain data properties keeps that
	// poller working unchanged for every kind of signal, including a
	// composite one from AbortSignal.any().
	jsObj *vm.PlainObject
}

// removeListenerLocked removes target from s.listeners. Caller must hold s.mu.
func (s *AbortSignal) removeListenerLocked(target *abortListener) {
	for i, l := range s.listeners {
		if l == target {
			s.listeners = append(s.listeners[:i], s.listeners[i+1:]...)
			return
		}
	}
}

// AbortController represents the controller object
type AbortController struct {
	signal *AbortSignal
}

// triggerAbort marks signal aborted with reason (a no-op if it's already
// aborted) and synchronously fires the "abort" event to every
// addEventListener listener (respecting `once`) and then to onabort, per
// the WHATWG spec's requirement that abort() dispatches synchronously.
func triggerAbort(vmInstance *vm.VM, signal *AbortSignal, reason vm.Value) {
	signal.mu.Lock()
	if signal.aborted {
		signal.mu.Unlock()
		return
	}
	signal.aborted = true
	signal.reason = reason
	if signal.jsObj != nil {
		signal.jsObj.SetOwn("aborted", vm.True)
		signal.jsObj.SetOwn("reason", reason)
	}
	listeners := make([]*abortListener, len(signal.listeners))
	copy(listeners, signal.listeners)
	onabort := signal.onabort
	var jsSelf vm.Value
	if signal.jsObj != nil {
		jsSelf = vm.NewValueFromPlainObject(signal.jsObj)
	}
	signal.mu.Unlock()

	if jsSelf.Type() == vm.TypeUndefined {
		return
	}

	event := createAbortEventValue(vmInstance, jsSelf)

	for _, l := range listeners {
		signal.mu.Lock()
		skip := l.removed
		if l.once && !skip {
			l.removed = true
			signal.removeListenerLocked(l)
		}
		signal.mu.Unlock()
		if skip {
			continue
		}
		callEventHandler(vmInstance, l.callback, jsSelf, event)
	}

	// NOTE: onabort always fires after every addEventListener listener,
	// regardless of the relative order they were registered/assigned in.
	// The spec instead fires listeners in registration order (onabort's
	// "position" is set by its first assignment), so
	// `addEventListener(A); signal.onabort = B; addEventListener(C)` fires
	// A, B, C here we fire A, C, B. Documented deviation - accepted to keep
	// this a single extra field instead of threading onabort through the
	// same ordered list as real listeners.
	if onabort.IsCallable() {
		callEventHandler(vmInstance, onabort, jsSelf, event)
	}
}

// callEventHandler invokes callback(event) with `this` bound to target, per
// the EventListener contract: a callable is called directly, an object goes
// through its handleEvent method.
func callEventHandler(vmInstance *vm.VM, callback vm.Value, target vm.Value, event vm.Value) {
	if callback.IsCallable() {
		_, _ = vmInstance.Call(callback, target, []vm.Value{event})
		return
	}
	if callback.Type() == vm.TypeObject {
		if he, exists := callback.AsPlainObject().GetOwn("handleEvent"); exists && he.IsCallable() {
			_, _ = vmInstance.Call(he, callback, []vm.Value{event})
		}
	}
}

// createAbortEventValue builds a minimal Event-like object for dispatch to
// "abort" listeners: type/target/currentTarget plus the no-op methods
// libraries commonly call defensively (preventDefault etc. - abort events
// are neither cancelable nor bubbling, so these are legitimately no-ops).
func createAbortEventValue(vmInstance *vm.VM, target vm.Value) vm.Value {
	evt := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	evt.SetOwn("type", vm.NewString("abort"))
	evt.SetOwn("target", target)
	evt.SetOwn("currentTarget", target)
	evt.SetOwn("bubbles", vm.False)
	evt.SetOwn("cancelable", vm.False)
	evt.SetOwn("composed", vm.False)
	evt.SetOwn("defaultPrevented", vm.False)
	evt.SetOwn("isTrusted", vm.True)
	evt.SetOwn("timeStamp", vm.NumberValue(float64(time.Now().UnixMilli())))

	noop := func(name string) vm.Value {
		return vm.NewNativeFunction(0, false, name, func(args []vm.Value) (vm.Value, error) {
			return vm.Undefined, nil
		})
	}
	evt.SetOwnNonEnumerable("preventDefault", noop("preventDefault"))
	evt.SetOwnNonEnumerable("stopPropagation", noop("stopPropagation"))
	evt.SetOwnNonEnumerable("stopImmediatePropagation", noop("stopImmediatePropagation"))

	return vm.NewValueFromPlainObject(evt)
}

// parseOnceOption reads the `once` flag out of addEventListener's optional
// third argument. A bare boolean there is the legacy `useCapture` parameter,
// not `once` (and capture is meaningless for AbortSignal's non-bubbling
// events), so only an options object's `once` property is honored.
func parseOnceOption(options vm.Value) bool {
	if options.Type() == vm.TypeObject {
		if once, exists := options.AsPlainObject().GetOwn("once"); exists {
			return once.IsTruthy()
		}
	}
	return false
}

func createAbortSignalObject(vmInstance *vm.VM, signal *AbortSignal, signalProto *vm.PlainObject) vm.Value {
	obj := vm.NewObject(vm.NewValueFromPlainObject(signalProto)).AsPlainObject()

	// Store the signal reference for internal use
	signalRef := signal

	signalRef.mu.Lock()
	signalRef.jsObj = obj
	aborted := signalRef.aborted
	reason := signalRef.reason
	signalRef.mu.Unlock()

	// aborted/reason: plain own data properties, kept in sync by
	// triggerAbort - see the doc comment on AbortSignal.jsObj for why these
	// aren't accessor properties.
	obj.SetOwn("aborted", boolToValue(aborted))
	obj.SetOwn("reason", reason)

	// throwIfAborted() - throws the signal's actual reason (per spec), not
	// a synthetic wrapper around it.
	obj.SetOwnNonEnumerable("throwIfAborted", vm.NewNativeFunction(0, false, "throwIfAborted", func(args []vm.Value) (vm.Value, error) {
		signalRef.mu.Lock()
		aborted := signalRef.aborted
		reason := signalRef.reason
		signalRef.mu.Unlock()

		if !aborted {
			return vm.Undefined, nil
		}
		if reason.Type() == vm.TypeUndefined {
			return vm.Undefined, vmInstance.NewExceptionError(newAbortErrorValue(vmInstance, "signal is aborted without reason"))
		}
		return vm.Undefined, vmInstance.NewExceptionError(reason)
	}))

	// addEventListener(type, listener, options?)
	obj.SetOwnNonEnumerable("addEventListener", vm.NewNativeFunction(2, true, "addEventListener", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, nil
		}
		if args[0].ToString() != "abort" {
			return vm.Undefined, nil
		}
		callback := args[1]
		if !callback.IsCallable() && callback.Type() != vm.TypeObject {
			return vm.Undefined, nil
		}
		once := false
		if len(args) > 2 {
			once = parseOnceOption(args[2])
		}

		signalRef.mu.Lock()
		for _, l := range signalRef.listeners {
			if l.callback.StrictlyEquals(callback) {
				// Adding an identical (type, listener) pair again is a no-op per spec.
				signalRef.mu.Unlock()
				return vm.Undefined, nil
			}
		}
		signalRef.listeners = append(signalRef.listeners, &abortListener{callback: callback, once: once})
		signalRef.mu.Unlock()

		return vm.Undefined, nil
	}))

	// removeEventListener(type, listener, options?)
	obj.SetOwnNonEnumerable("removeEventListener", vm.NewNativeFunction(2, true, "removeEventListener", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, nil
		}
		if args[0].ToString() != "abort" {
			return vm.Undefined, nil
		}
		callback := args[1]

		signalRef.mu.Lock()
		for i, l := range signalRef.listeners {
			if l.callback.StrictlyEquals(callback) {
				l.removed = true
				signalRef.listeners = append(signalRef.listeners[:i], signalRef.listeners[i+1:]...)
				break
			}
		}
		signalRef.mu.Unlock()

		return vm.Undefined, nil
	}))

	// onabort - the event-handler IDL attribute; starts out null. Not
	// enumerable: real engines put it on the prototype (never own-property
	// enumerable), which matters because JSON.stringify(signal) must not
	// start emitting it just because we implement it as an own accessor.
	accEnumerable, accConfigurable := false, true
	obj.DefineAccessorProperty("onabort",
		vm.NewNativeFunction(0, false, "get onabort", func(args []vm.Value) (vm.Value, error) {
			signalRef.mu.Lock()
			h := signalRef.onabort
			signalRef.mu.Unlock()
			return h, nil
		}), true,
		vm.NewNativeFunction(1, false, "set onabort", func(args []vm.Value) (vm.Value, error) {
			v := vm.Null
			if len(args) > 0 {
				v = args[0]
			}
			signalRef.mu.Lock()
			signalRef.onabort = v
			signalRef.mu.Unlock()
			return vm.Undefined, nil
		}), true,
		&accEnumerable, &accConfigurable,
	)

	return vm.NewValueFromPlainObject(obj)
}

func createAbortControllerObject(vmInstance *vm.VM, controller *AbortController, controllerProto *vm.PlainObject, signalProto *vm.PlainObject) vm.Value {
	obj := vm.NewObject(vm.NewValueFromPlainObject(controllerProto)).AsPlainObject()

	// Create the signal object
	signalObj := createAbortSignalObject(vmInstance, controller.signal, signalProto)

	// signal property
	obj.SetOwn("signal", signalObj)

	// abort(reason?) method
	obj.SetOwnNonEnumerable("abort", vm.NewNativeFunction(1, false, "abort", func(args []vm.Value) (vm.Value, error) {
		var reason vm.Value
		if len(args) > 0 && !args[0].IsUndefined() {
			reason = args[0]
		} else {
			reason = newAbortErrorValue(vmInstance, "signal is aborted without reason")
		}

		triggerAbort(vmInstance, controller.signal, reason)

		return vm.Undefined, nil
	}))

	return vm.NewValueFromPlainObject(obj)
}

// AbortError represents an abort error
type AbortError struct {
	Message string
}

func (e *AbortError) Error() string {
	return "AbortError: " + e.Message
}
