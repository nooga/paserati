package builtins

import (
	"sync"

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
		WithProperty("throwIfAborted", types.NewSimpleFunction([]types.Type{}, types.Undefined)).
		WithProperty("addEventListener", types.NewSimpleFunction([]types.Type{types.String, types.Any}, types.Undefined)).
		WithProperty("removeEventListener", types.NewSimpleFunction([]types.Type{types.String, types.Any}, types.Undefined))

	// AbortSignal static methods
	abortSignalConstructorType := types.NewObjectType().
		WithProperty("abort", types.NewSimpleFunction([]types.Type{}, abortSignalType)).
		WithProperty("timeout", types.NewSimpleFunction([]types.Type{types.Number}, abortSignalType)).
		WithProperty("any", types.NewSimpleFunction([]types.Type{types.Any}, abortSignalType)). // signals array
		WithProperty("prototype", abortSignalType)

	if err := ctx.DefineGlobal("AbortSignal", abortSignalConstructorType); err != nil {
		return err
	}

	// AbortController type
	abortControllerType := types.NewObjectType().
		WithProperty("signal", abortSignalType).
		WithProperty("abort", types.NewSimpleFunction([]types.Type{}, types.Undefined))

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
		if len(args) > 0 {
			reason = args[0]
		} else {
			// Default reason is DOMException with name "AbortError"
			reason = vm.NewString("AbortError: signal is aborted without reason")
		}
		signal := &AbortSignal{
			aborted:   true,
			reason:    reason,
			listeners: make([]vm.Value, 0),
		}
		return createAbortSignalObject(vmInstance, signal, signalProto), nil
	}))

	// AbortSignal.timeout(ms) - creates a signal that aborts after timeout
	signalConstructor.SetOwnNonEnumerable("timeout", vm.NewNativeFunction(1, false, "timeout", func(args []vm.Value) (vm.Value, error) {
		// For now, return a non-aborted signal (timeout would need async runtime support)
		// This is a simplified implementation
		signal := &AbortSignal{
			aborted:   false,
			reason:    vm.Undefined,
			listeners: make([]vm.Value, 0),
		}
		return createAbortSignalObject(vmInstance, signal, signalProto), nil
	}))

	// AbortSignal.any(signals) - creates a signal that aborts when any input signal aborts
	signalConstructor.SetOwnNonEnumerable("any", vm.NewNativeFunction(1, false, "any", func(args []vm.Value) (vm.Value, error) {
		signal := &AbortSignal{
			aborted:   false,
			reason:    vm.Undefined,
			listeners: make([]vm.Value, 0),
		}
		// Check if any input signal is already aborted
		if len(args) > 0 {
			if arr := args[0].AsArray(); arr != nil {
				for i := 0; i < arr.Length(); i++ {
					elem := arr.Get(i)
					if obj := elem.AsPlainObject(); obj != nil {
						if aborted, exists := obj.GetOwn("aborted"); exists && aborted.IsBoolean() && aborted.AsBoolean() {
							signal.aborted = true
							if reason, exists := obj.GetOwn("reason"); exists {
								signal.reason = reason
							}
							break
						}
					}
				}
			}
		}
		return createAbortSignalObject(vmInstance, signal, signalProto), nil
	}))

	signalConstructor.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(signalProto))

	if err := ctx.DefineGlobal("AbortSignal", vm.NewValueFromPlainObject(signalConstructor)); err != nil {
		return err
	}

	// Create AbortController.prototype
	controllerProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// AbortController constructor
	controllerConstructorFn := func(args []vm.Value) (vm.Value, error) {
		signal := &AbortSignal{
			aborted:   false,
			reason:    vm.Undefined,
			listeners: make([]vm.Value, 0),
		}
		controller := &AbortController{
			signal: signal,
		}
		return createAbortControllerObject(vmInstance, controller, controllerProto, signalProto), nil
	}

	controllerConstructor := vm.NewConstructorWithProps(0, false, "AbortController", controllerConstructorFn)
	if ctorProps := controllerConstructor.AsNativeFunctionWithProps(); ctorProps != nil {
		ctorProps.Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(controllerProto))
	}

	controllerProto.SetOwnNonEnumerable("constructor", controllerConstructor)

	return ctx.DefineGlobal("AbortController", controllerConstructor)
}

// AbortSignal represents the signal object
type AbortSignal struct {
	mu        sync.Mutex
	aborted   bool
	reason    vm.Value
	listeners []vm.Value
}

// AbortController represents the controller object
type AbortController struct {
	signal *AbortSignal
}

// Abort aborts the signal with an optional reason
func (s *AbortSignal) Abort(reason vm.Value) {
	s.mu.Lock()
	if s.aborted {
		s.mu.Unlock()
		return
	}
	s.aborted = true
	s.reason = reason
	listeners := make([]vm.Value, len(s.listeners))
	copy(listeners, s.listeners)
	s.mu.Unlock()

	// Note: We don't call listeners here because we don't have access to the VM
	// The listeners would need to be called from the VM context
}

func createAbortSignalObject(vmInstance *vm.VM, signal *AbortSignal, _ *vm.PlainObject) vm.Value {
	obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Store the signal reference for internal use
	signalRef := signal

	// aborted property (getter-like behavior via direct access)
	obj.SetOwn("aborted", boolToValue(signal.aborted))
	obj.SetOwn("reason", signal.reason)

	// throwIfAborted() - throws if the signal is aborted
	obj.SetOwnNonEnumerable("throwIfAborted", vm.NewNativeFunction(0, false, "throwIfAborted", func(args []vm.Value) (vm.Value, error) {
		signalRef.mu.Lock()
		aborted := signalRef.aborted
		reason := signalRef.reason
		signalRef.mu.Unlock()

		if aborted {
			if reason.Type() == vm.TypeUndefined {
				return vm.Undefined, &AbortError{Message: "signal is aborted without reason"}
			}
			return vm.Undefined, &AbortError{Message: reason.ToString()}
		}
		return vm.Undefined, nil
	}))

	// addEventListener(type, listener)
	obj.SetOwnNonEnumerable("addEventListener", vm.NewNativeFunction(2, false, "addEventListener", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, nil
		}
		eventType := args[0].ToString()
		if eventType == "abort" {
			signalRef.mu.Lock()
			signalRef.listeners = append(signalRef.listeners, args[1])
			signalRef.mu.Unlock()
		}
		return vm.Undefined, nil
	}))

	// removeEventListener(type, listener)
	obj.SetOwnNonEnumerable("removeEventListener", vm.NewNativeFunction(2, false, "removeEventListener", func(args []vm.Value) (vm.Value, error) {
		// Simplified - just return undefined
		return vm.Undefined, nil
	}))

	return vm.NewValueFromPlainObject(obj)
}

func createAbortControllerObject(vmInstance *vm.VM, controller *AbortController, _ *vm.PlainObject, signalProto *vm.PlainObject) vm.Value {
	obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Create the signal object
	signalObj := createAbortSignalObject(vmInstance, controller.signal, signalProto)
	signalPlain := signalObj.AsPlainObject()

	// signal property
	obj.SetOwn("signal", signalObj)

	// abort(reason?) method
	obj.SetOwnNonEnumerable("abort", vm.NewNativeFunction(1, false, "abort", func(args []vm.Value) (vm.Value, error) {
		var reason vm.Value
		if len(args) > 0 {
			reason = args[0]
		} else {
			reason = vm.NewString("AbortError: signal is aborted without reason")
		}

		controller.signal.mu.Lock()
		if !controller.signal.aborted {
			controller.signal.aborted = true
			controller.signal.reason = reason
			// Update the signal object's properties
			if signalPlain != nil {
				signalPlain.SetOwn("aborted", vm.True)
				signalPlain.SetOwn("reason", reason)
			}
		}
		controller.signal.mu.Unlock()

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
