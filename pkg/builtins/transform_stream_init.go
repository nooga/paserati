package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// PriorityTransformStream places TransformStream after both ReadableStream
// (185) and WritableStream (186), since it composes one of each (#413).
const PriorityTransformStream = 187

// TransformStreamInitializer implements a minimal TransformStream: a
// ReadableStream/WritableStream pair joined by a transform algorithm, per
// the WHATWG Streams spec's `Transformer` interface. This is what real,
// unmodified SDK code (the AWS SDK's @smithy/core checksum stream, its
// middleware-websocket package's own `class X extends TransformStream`)
// actually reaches for (#413) - ReadableStream (#205) alone isn't enough.
//
//	new TransformStream({
//	  start(controller),                    // optional
//	  transform(chunk, controller),         // optional - default: enqueue chunk unchanged
//	  flush(controller),                    // optional
//	}) -> {
//	  readable: ReadableStream,
//	  writable: WritableStream,
//	}
//
// controller (TransformStreamDefaultController) exposes enqueue(chunk),
// error(reason) and terminate().
//
// Wiring: every chunk written to `.writable` runs the transform algorithm,
// which typically calls controller.enqueue() to push output chunks onto
// `.readable`; closing `.writable` runs flush() (if given) and then closes
// `.readable`; erroring or terminating propagates to both sides. Writes are
// processed strictly in order (WritableStream's own tail-chaining, see
// writable_stream_init.go) so transform() sees chunks in write() order.
//
// Simplifications: no readableType/writableType (BYOB) support, and
// desiredSize on the controller is a fixed placeholder rather than tracking
// real queue occupancy - matching this package's established
// ReadableStream/WritableStream scope notes.
type TransformStreamInitializer struct{}

func (t *TransformStreamInitializer) Name() string  { return "TransformStream" }
func (t *TransformStreamInitializer) Priority() int { return PriorityTransformStream }

func (t *TransformStreamInitializer) InitTypes(ctx *TypeContext) error {
	// readable/writable are kept Any-typed: ReadableStream/WritableStream's
	// own declared types live in separate initializers, and Any avoids
	// coupling this constructor's type to their exact structural shape.
	streamType := types.NewObjectType().
		WithProperty("readable", types.Any).
		WithProperty("writable", types.Any)

	streamCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, streamType).
		WithSimpleCallSignature([]types.Type{types.Any}, streamType).
		WithSimpleConstructSignature([]types.Type{}, streamType).
		WithSimpleConstructSignature([]types.Type{types.Any}, streamType).
		WithProperty("prototype", streamType)

	return ctx.DefineGlobal("TransformStream", streamCtorType)
}

func (t *TransformStreamInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	streamProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	transformStreamProto = streamProto

	ctorFn := func(args []vm.Value) (vm.Value, error) {
		var transformer vm.Value = vm.Undefined
		if len(args) > 0 {
			transformer = args[0]
		}

		readableState := newReadableStreamState(vmInstance)
		writableState := newWritableStreamState(vmInstance)

		controllerVal := createTransformStreamControllerObject(vmInstance, readableState, writableState)

		// Every chunk written to .writable runs the transform algorithm; by
		// default (no transform() given) it just enqueues the chunk
		// unchanged, per spec.
		writableState.goWrite = func(chunk vm.Value) vm.Value {
			if transformFn, err := vmInstance.GetProperty(transformer, "transform"); err == nil && transformFn.IsCallable() {
				return callAlgorithm(vmInstance, transformFn, transformer, []vm.Value{chunk, controllerVal})
			}
			readableState.enqueue(chunk)
			return vmInstance.NewResolvedPromise(vm.Undefined)
		}

		// Closing .writable runs flush() (if given), then closes .readable.
		writableState.goClose = func() vm.Value {
			flushVal := callAlgorithmIfPresent(vmInstance, transformer, "flush", []vm.Value{controllerVal})
			resultVal := vmInstance.NewPendingPromise()
			resultPromise := resultVal.AsPromise()
			vmInstance.AddPromiseReaction(flushVal, true, func(vm.Value) {
				readableState.close()
				vmInstance.ResolvePromise(resultPromise, vm.Undefined)
			})
			vmInstance.AddPromiseReaction(flushVal, false, func(r vm.Value) {
				readableState.errorOut(r)
				vmInstance.RejectPromise(resultPromise, r)
			})
			return resultVal
		}

		// Aborting .writable (from outside, e.g. writer.abort()) errors the
		// readable side too, per spec.
		writableState.goAbort = func(reason vm.Value) vm.Value {
			readableState.errorOut(reason)
			return vmInstance.NewResolvedPromise(vm.Undefined)
		}

		// Cancelling .readable errors the writable side, per spec (no more
		// chunks can usefully be transformed once the consumer walked away).
		readableState.goCancel = func(reason vm.Value) vm.Value {
			writableState.errorOut(reason)
			return vmInstance.NewResolvedPromise(vm.Undefined)
		}

		readableVal := createReadableStreamObject(vmInstance, readableState, readableStreamProto, readableStreamReaderProto)
		writableVal := createWritableStreamObject(vmInstance, writableState, writableStreamProto)

		obj := vm.NewObject(vm.NewValueFromPlainObject(streamProto)).AsPlainObject()
		obj.SetOwnNonEnumerable("readable", readableVal)
		obj.SetOwnNonEnumerable("writable", writableVal)

		if transformer.Type() == vm.TypeObject || transformer.Type() == vm.TypeDictObject {
			if startFn, err := vmInstance.GetProperty(transformer, "start"); err == nil && startFn.IsCallable() {
				if _, callErr := vmInstance.Call(startFn, transformer, []vm.Value{controllerVal}); callErr != nil {
					return vm.Undefined, callErr
				}
			}
		}

		return vm.NewValueFromPlainObject(obj), nil
	}

	ctor := vm.NewConstructorWithProps(0, false, "TransformStream", ctorFn)
	if ctor.Type() == vm.TypeNativeFunctionWithProps {
		ctor.AsNativeFunctionWithProps().Properties.DefineFixedProperty("prototype", vm.NewValueFromPlainObject(streamProto))
	}
	streamProto.SetOwnNonEnumerable("constructor", ctor)

	return ctx.DefineGlobal("TransformStream", ctor)
}

// Package-level prototype, set once during InitRuntime - see the identical
// convention on readableStreamProto/writableStreamProto.
var transformStreamProto *vm.PlainObject

// callAlgorithmIfPresent looks up an optional hook (transformer.flush, etc.)
// and calls it if present and callable, wrapping the result the same way
// callAlgorithm does; if absent, it's a no-op that resolves immediately -
// the correct default per spec for every optional Transformer hook.
func callAlgorithmIfPresent(vmInstance *vm.VM, obj vm.Value, name string, args []vm.Value) vm.Value {
	if obj.Type() != vm.TypeObject && obj.Type() != vm.TypeDictObject {
		return vmInstance.NewResolvedPromise(vm.Undefined)
	}
	fn, err := vmInstance.GetProperty(obj, name)
	if err != nil || !fn.IsCallable() {
		return vmInstance.NewResolvedPromise(vm.Undefined)
	}
	return callAlgorithm(vmInstance, fn, obj, args)
}

// createTransformStreamControllerObject builds the
// TransformStreamDefaultController handed to transformer.start()/
// transform()/flush(): enqueue(chunk) pushes onto the readable side,
// error(reason) errors both sides, terminate() closes the readable side and
// errors the writable side (per spec, "the stream is being closed").
func createTransformStreamControllerObject(vmInstance *vm.VM, readableState *readableStreamState, writableState *writableStreamState) vm.Value {
	obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// A real accessor (not a value snapshotted at controller-creation time,
	// which would go stale after the first enqueue/close/error) - backed by
	// the readable side's own queue via readableStreamState.desiredSizeValue.
	desiredSizeGetter := vm.NewNativeFunction(0, false, "desiredSize", func(args []vm.Value) (vm.Value, error) {
		return readableState.desiredSizeValue(1), nil
	})
	enumerable, configurable := true, true
	obj.DefineAccessorProperty("desiredSize", desiredSizeGetter, true, vm.Undefined, false, &enumerable, &configurable)

	obj.SetOwnNonEnumerable("enqueue", vm.NewNativeFunction(1, false, "enqueue", func(args []vm.Value) (vm.Value, error) {
		var chunk vm.Value = vm.Undefined
		if len(args) > 0 {
			chunk = args[0]
		}
		readableState.enqueue(chunk)
		return vm.Undefined, nil
	}))

	obj.SetOwnNonEnumerable("error", vm.NewNativeFunction(1, false, "error", func(args []vm.Value) (vm.Value, error) {
		var reason vm.Value = vm.Undefined
		if len(args) > 0 {
			reason = args[0]
		}
		readableState.errorOut(reason)
		writableState.errorOut(reason)
		return vm.Undefined, nil
	}))

	obj.SetOwnNonEnumerable("terminate", vm.NewNativeFunction(0, false, "terminate", func(args []vm.Value) (vm.Value, error) {
		readableState.close()
		reason := errorValueFromGoError(vmInstance.NewTypeError("The transform stream has been terminated"))
		writableState.errorOut(reason)
		return vm.Undefined, nil
	}))

	return vm.NewValueFromPlainObject(obj)
}
