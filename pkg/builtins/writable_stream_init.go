package builtins

import (
	"sync"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// PriorityWritableStream places WritableStream right after ReadableStream
// (185), so TransformStream (187, #413) can compose both.
const PriorityWritableStream = 186

// WritableStreamInitializer implements a minimal WritableStream /
// WritableStreamDefaultWriter, sized the same way ReadableStreamInitializer
// is (see that file's doc comment, #205): enough surface for real streaming
// consumers - here, primarily TransformStream.writable (#413) and any real
// SDK code that does `stream.getWriter().write(chunk)`.
//
//	new WritableStream(underlyingSink) -> {
//	  locked: boolean,
//	  getWriter() -> {
//	    write(chunk): Promise<void>,
//	    close(): Promise<void>,
//	    abort(reason?): Promise<void>,
//	    releaseLock(): void,
//	    ready: Promise<void>,
//	    closed: Promise<void>,
//	    desiredSize: number | null,
//	  },
//	  abort(reason?): Promise<void>,
//	}
//
// underlyingSink may implement start(controller)/write(chunk, controller)/
// close()/abort(reason). controller exposes error(reason).
//
// Simplifications vs. the full spec (matching ReadableStream's own scope
// note): no real highWaterMark/queue-size backpressure accounting -
// desiredSize is a plain 0-or-1 signal (0 while a write/close is in flight,
// 1 once it settles) and `ready` simply mirrors the in-flight op's own
// promise. Writes and the eventual close are still strictly ordered (see
// writableStreamState.tail below) - that ordering is what TransformStream's
// transform()/flush() correctness actually depends on, unlike the exact
// backpressure numbers.
type WritableStreamInitializer struct{}

func (w *WritableStreamInitializer) Name() string  { return "WritableStream" }
func (w *WritableStreamInitializer) Priority() int { return PriorityWritableStream }

func (w *WritableStreamInitializer) InitTypes(ctx *TypeContext) error {
	writerType := types.NewObjectType().
		WithProperty("write", types.NewOptionalFunction([]types.Type{types.Any}, types.Any, []bool{true})).
		WithProperty("close", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("abort", types.NewOptionalFunction([]types.Type{types.Any}, types.Any, []bool{true})).
		WithProperty("releaseLock", types.NewSimpleFunction([]types.Type{}, types.Undefined)).
		WithProperty("ready", types.Any).
		WithProperty("closed", types.Any).
		WithProperty("desiredSize", types.Any)

	streamType := types.NewObjectType().
		WithProperty("locked", types.Boolean).
		WithProperty("getWriter", types.NewSimpleFunction([]types.Type{}, writerType)).
		WithProperty("abort", types.NewOptionalFunction([]types.Type{types.Any}, types.Any, []bool{true}))

	streamCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, streamType).
		WithSimpleCallSignature([]types.Type{types.Any}, streamType).
		WithSimpleConstructSignature([]types.Type{}, streamType).
		WithSimpleConstructSignature([]types.Type{types.Any}, streamType).
		WithProperty("prototype", streamType)

	return ctx.DefineGlobal("WritableStream", streamCtorType)
}

func (w *WritableStreamInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	streamProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	writerProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Stashed so TransformStream (transform_stream_init.go) can build a
	// writable side sharing these prototypes without a JS underlyingSink,
	// mirroring readableStreamProto/readableStreamReaderProto's convention.
	writableStreamProto = streamProto
	writableStreamWriterProto = writerProto

	ctorFn := func(args []vm.Value) (vm.Value, error) {
		state := newWritableStreamState(vmInstance)
		if len(args) > 0 {
			state.underlyingSink = args[0]
		}
		streamVal := createWritableStreamObject(vmInstance, state, streamProto)

		if state.underlyingSink.Type() == vm.TypeObject || state.underlyingSink.Type() == vm.TypeDictObject {
			if startFn, err := vmInstance.GetProperty(state.underlyingSink, "start"); err == nil && startFn.IsCallable() {
				controllerVal := createWritableStreamControllerObject(vmInstance, state)
				if _, callErr := vmInstance.Call(startFn, state.underlyingSink, []vm.Value{controllerVal}); callErr != nil {
					return vm.Undefined, callErr
				}
			}
		}

		return streamVal, nil
	}

	ctor := vm.NewConstructorWithProps(1, false, "WritableStream", ctorFn)
	if ctor.Type() == vm.TypeNativeFunctionWithProps {
		ctor.AsNativeFunctionWithProps().Properties.DefineFixedProperty("prototype", vm.NewValueFromPlainObject(streamProto))
	}
	streamProto.SetOwnNonEnumerable("constructor", ctor)

	return ctx.DefineGlobal("WritableStream", ctor)
}

// Package-level prototypes, set once during InitRuntime - see the identical
// convention on readableStreamProto/readableStreamReaderProto.
var (
	writableStreamProto       *vm.PlainObject
	writableStreamWriterProto *vm.PlainObject
)

// callAlgorithm invokes a JS hook function and wraps whatever it returns (a
// plain value, undefined, or a thenable) into a Promise<value> - i.e. the
// spec's implicit "Promise.resolve(fn())" around every sink/transformer
// algorithm. vmInstance.ResolvePromise already does the thenable-chaining
// work (and settles synchronously for a plain value), so this is a thin
// wrapper. Shared with transform_stream_init.go (#413).
func callAlgorithm(vmInstance *vm.VM, fn vm.Value, thisArg vm.Value, args []vm.Value) vm.Value {
	if !fn.IsCallable() {
		return vmInstance.NewResolvedPromise(vm.Undefined)
	}
	result, err := vmInstance.Call(fn, thisArg, args)
	if err != nil {
		return vmInstance.NewRejectedPromise(errorValueFromGoError(err))
	}
	p := vmInstance.NewPendingPromise()
	vmInstance.ResolvePromise(p.AsPromise(), result)
	return p
}

// writableStreamState is the internal, non-JS-visible state backing a
// WritableStream. Unlike readableStreamState, nothing here is fed from a
// background goroutine - every entry point (write/close/abort) is only ever
// reached synchronously from a JS native-function call, so the mutex here is
// purely defensive/for-consistency-with-the-rest-of-the-package, not load
// bearing the way it is for ReadableStream's goroutine-fed case.
type writableStreamState struct {
	mu sync.Mutex

	vmInstance *vm.VM
	streamObj  *vm.PlainObject
	writerObj  *vm.PlainObject // the current writer, kept in sync with ready/closed/desiredSize; nil once released

	locked bool

	closing    bool // close() called, not yet settled
	closed     bool
	errored    bool
	errorValue vm.Value

	underlyingSink vm.Value // Undefined if none was given

	// tail is the promise the next scheduled operation (write or close)
	// chains onto, so operations reach the sink/goWrite hook in the exact
	// order write()/close() were called - see scheduleOp.
	tail vm.Value

	// closedVal is the writer's `closed` promise, lazily created by
	// currentClosedVal() the first time anything needs it (a getWriter()
	// call, or an early error/abort/close before any writer exists).
	closedVal vm.Value

	// goWrite/goClose/goAbort let a Go-authored sink (TransformStream's
	// writable side) hook write()/close()/abort() without a JS
	// underlyingSink object - mirrors readableStreamState's goPull/goCancel.
	goWrite func(chunk vm.Value) vm.Value
	goClose func() vm.Value
	goAbort func(reason vm.Value) vm.Value
}

func newWritableStreamState(vmInstance *vm.VM) *writableStreamState {
	return &writableStreamState{
		vmInstance:     vmInstance,
		underlyingSink: vm.Undefined,
		tail:           vmInstance.NewResolvedPromise(vm.Undefined),
	}
}

// syncWriterProps mirrors state's current ready/closed/desiredSize onto the
// live writer object, the same "keep a plain own property in sync by hand"
// convention ReadableStream uses for `locked` (see createReadableStreamObject).
func (s *writableStreamState) syncWriterProps(ready, closed vm.Value, desiredSize vm.Value) {
	if s.writerObj == nil {
		return
	}
	s.writerObj.SetOwn("ready", ready)
	s.writerObj.SetOwn("closed", closed)
	s.writerObj.SetOwn("desiredSize", desiredSize)
}

// scheduleOp chains run onto the state's op tail so writes (and the eventual
// close) reach the sink in call order, and returns a promise settling with
// run's own outcome. Must be called with s.mu held; it releases the lock
// itself once it has snapshotted/replaced s.tail.
func (s *writableStreamState) scheduleOp(run func() vm.Value) vm.Value {
	resultVal := s.vmInstance.NewPendingPromise()
	resultPromise := resultVal.AsPromise()
	prevTail := s.tail
	s.tail = resultVal
	s.mu.Unlock()

	s.vmInstance.AddPromiseReaction(prevTail, true, func(vm.Value) {
		opVal := run()
		s.vmInstance.AddPromiseReaction(opVal, true, func(v vm.Value) {
			s.vmInstance.ResolvePromise(resultPromise, v)
		})
		s.vmInstance.AddPromiseReaction(opVal, false, func(r vm.Value) {
			s.errorOut(r)
			s.vmInstance.RejectPromise(resultPromise, r)
		})
	})
	s.vmInstance.AddPromiseReaction(prevTail, false, func(r vm.Value) {
		// A prior op in the chain already failed (and already errored the
		// stream via the reaction above) - this one never runs.
		s.vmInstance.RejectPromise(resultPromise, r)
	})

	return resultVal
}

// performWrite calls the sink/goWrite hook for one chunk.
func (s *writableStreamState) performWrite(chunk vm.Value) vm.Value {
	if s.goWrite != nil {
		return s.goWrite(chunk)
	}
	sink := s.underlyingSink
	if sink.Type() != vm.TypeObject && sink.Type() != vm.TypeDictObject {
		return s.vmInstance.NewResolvedPromise(vm.Undefined)
	}
	writeFn, err := s.vmInstance.GetProperty(sink, "write")
	if err != nil || !writeFn.IsCallable() {
		return s.vmInstance.NewResolvedPromise(vm.Undefined)
	}
	controllerVal := createWritableStreamControllerObject(s.vmInstance, s)
	return callAlgorithm(s.vmInstance, writeFn, sink, []vm.Value{chunk, controllerVal})
}

// performClose calls the sink/goClose hook.
func (s *writableStreamState) performClose() vm.Value {
	if s.goClose != nil {
		return s.goClose()
	}
	sink := s.underlyingSink
	if sink.Type() != vm.TypeObject && sink.Type() != vm.TypeDictObject {
		return s.vmInstance.NewResolvedPromise(vm.Undefined)
	}
	closeFn, err := s.vmInstance.GetProperty(sink, "close")
	if err != nil || !closeFn.IsCallable() {
		return s.vmInstance.NewResolvedPromise(vm.Undefined)
	}
	return callAlgorithm(s.vmInstance, closeFn, sink, nil)
}

// performAbort calls the sink/goAbort hook.
func (s *writableStreamState) performAbort(reason vm.Value) vm.Value {
	if s.goAbort != nil {
		return s.goAbort(reason)
	}
	sink := s.underlyingSink
	if sink.Type() != vm.TypeObject && sink.Type() != vm.TypeDictObject {
		return s.vmInstance.NewResolvedPromise(vm.Undefined)
	}
	abortFn, err := s.vmInstance.GetProperty(sink, "abort")
	if err != nil || !abortFn.IsCallable() {
		return s.vmInstance.NewResolvedPromise(vm.Undefined)
	}
	return callAlgorithm(s.vmInstance, abortFn, sink, []vm.Value{reason})
}

// write implements writer.write(chunk).
func (s *writableStreamState) write(chunk vm.Value) vm.Value {
	s.mu.Lock()
	if s.errored {
		reason := s.errorValue
		s.mu.Unlock()
		return s.vmInstance.NewRejectedPromise(reason)
	}
	if s.closing || s.closed {
		s.mu.Unlock()
		reason := errorValueFromGoError(s.vmInstance.NewTypeError("Cannot write to a WritableStream that is closing or has already been closed"))
		return s.vmInstance.NewRejectedPromise(reason)
	}
	resultVal := s.scheduleOp(func() vm.Value { return s.performWrite(chunk) })

	s.mu.Lock()
	s.syncWriterProps(resultVal, s.currentClosedVal(), vm.NumberValue(0))
	s.mu.Unlock()

	// Once this write settles, desiredSize goes back to "ready for more"
	// (approximate backpressure - see the type doc comment's scope note).
	s.vmInstance.AddPromiseReaction(resultVal, true, func(vm.Value) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.errored && !s.closed {
			s.syncWriterProps(s.vmInstance.NewResolvedPromise(vm.Undefined), s.currentClosedVal(), vm.NumberValue(1))
		}
	})

	return resultVal
}

// currentClosedVal returns the writer's current `closed` value. Must be
// called with s.mu held. Lazily created so a writer whose stream was never
// closed/errored doesn't need one until asked.
func (s *writableStreamState) currentClosedVal() vm.Value {
	if s.closedVal.Type() == 0 {
		s.closedVal = s.vmInstance.NewPendingPromise()
	}
	return s.closedVal
}

// close implements writer.close().
func (s *writableStreamState) close() vm.Value {
	s.mu.Lock()
	if s.errored {
		reason := s.errorValue
		s.mu.Unlock()
		return s.vmInstance.NewRejectedPromise(reason)
	}
	if s.closing || s.closed {
		closedVal := s.currentClosedVal()
		s.mu.Unlock()
		return closedVal
	}
	s.closing = true
	closedVal := s.currentClosedVal()
	// scheduleOp expects s.mu held and releases it itself - kept locked
	// across the two statements above rather than re-locking, same as write().
	resultVal := s.scheduleOp(func() vm.Value { return s.performClose() })

	s.vmInstance.AddPromiseReaction(resultVal, true, func(vm.Value) {
		s.mu.Lock()
		s.closing = false
		s.closed = true
		obj := s.streamObj
		s.syncWriterProps(s.vmInstance.NewResolvedPromise(vm.Undefined), closedVal, vm.NumberValue(0))
		s.mu.Unlock()
		if obj != nil {
			obj.SetOwn("locked", s.snapshotLocked())
		}
		s.vmInstance.ResolvePromise(closedVal.AsPromise(), vm.Undefined)
	})
	s.vmInstance.AddPromiseReaction(resultVal, false, func(r vm.Value) {
		s.mu.Lock()
		s.closing = false
		s.mu.Unlock()
		s.vmInstance.RejectPromise(closedVal.AsPromise(), r)
	})

	return closedVal
}

// snapshotLocked reports the `locked` value to publish on the stream object
// after close settles - always false would be wrong if a writer released
// itself mid-close, so this reads the live flag under lock.
func (s *writableStreamState) snapshotLocked() vm.Value {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return vm.True
	}
	return vm.False
}

// abort implements writer.abort(reason)/stream.abort(reason): immediately
// errors the stream (rejecting the closed promise and any writes still
// chained onto tail) and forwards to the sink/goAbort hook.
func (s *writableStreamState) abort(reason vm.Value) vm.Value {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return s.vmInstance.NewResolvedPromise(vm.Undefined)
	}
	if s.errored {
		s.mu.Unlock()
		return s.vmInstance.NewResolvedPromise(vm.Undefined)
	}
	s.errored = true
	s.errorValue = reason
	closedVal := s.currentClosedVal()
	s.mu.Unlock()

	s.vmInstance.RejectPromise(closedVal.AsPromise(), reason)

	return s.performAbort(reason)
}

// errorOut marks the stream errored (e.g. from
// WritableStreamDefaultController.error()) without invoking the sink's
// abort() hook - matching the spec distinction between controller.error()
// (an internal signal) and an explicit abort() call.
func (s *writableStreamState) errorOut(reason vm.Value) {
	s.mu.Lock()
	if s.closed || s.errored {
		s.mu.Unlock()
		return
	}
	s.errored = true
	s.errorValue = reason
	closedVal := s.currentClosedVal()
	s.mu.Unlock()

	s.vmInstance.RejectPromise(closedVal.AsPromise(), reason)
}

// release implements writer.releaseLock().
func (s *writableStreamState) release() {
	s.mu.Lock()
	s.locked = false
	s.writerObj = nil
	obj := s.streamObj
	s.mu.Unlock()
	if obj != nil {
		obj.SetOwn("locked", vm.False)
	}
}

func createWritableStreamObject(vmInstance *vm.VM, state *writableStreamState, streamProto *vm.PlainObject) vm.Value {
	obj := vm.NewObject(vm.NewValueFromPlainObject(streamProto)).AsPlainObject()
	state.streamObj = obj
	obj.SetOwn("locked", vm.False)

	obj.SetOwnNonEnumerable("getWriter", vm.NewNativeFunction(0, false, "getWriter", func(args []vm.Value) (vm.Value, error) {
		state.mu.Lock()
		if state.locked {
			state.mu.Unlock()
			return vm.Undefined, vmInstance.NewTypeError("WritableStream is already locked to a writer")
		}
		state.locked = true
		state.mu.Unlock()
		obj.SetOwn("locked", vm.True)
		return createWritableStreamWriterObject(state, writableStreamWriterProto), nil
	}))

	obj.SetOwnNonEnumerable("abort", vm.NewNativeFunction(1, false, "abort", func(args []vm.Value) (vm.Value, error) {
		var reason vm.Value = vm.Undefined
		if len(args) > 0 {
			reason = args[0]
		}
		return state.abort(reason), nil
	}))

	return vm.NewValueFromPlainObject(obj)
}

func createWritableStreamWriterObject(state *writableStreamState, writerProto *vm.PlainObject) vm.Value {
	obj := vm.NewObject(vm.NewValueFromPlainObject(writerProto)).AsPlainObject()

	state.mu.Lock()
	state.writerObj = obj
	closedVal := state.currentClosedVal()
	readyVal := state.tail
	desiredSize := vm.NumberValue(1)
	if state.closed {
		desiredSize = vm.NumberValue(0)
	} else if state.errored {
		desiredSize = vm.Null
	}
	state.mu.Unlock()

	obj.SetOwn("ready", readyVal)
	obj.SetOwn("closed", closedVal)
	obj.SetOwn("desiredSize", desiredSize)

	obj.SetOwnNonEnumerable("write", vm.NewNativeFunction(1, false, "write", func(args []vm.Value) (vm.Value, error) {
		var chunk vm.Value = vm.Undefined
		if len(args) > 0 {
			chunk = args[0]
		}
		return state.write(chunk), nil
	}))

	obj.SetOwnNonEnumerable("close", vm.NewNativeFunction(0, false, "close", func(args []vm.Value) (vm.Value, error) {
		return state.close(), nil
	}))

	obj.SetOwnNonEnumerable("abort", vm.NewNativeFunction(1, false, "abort", func(args []vm.Value) (vm.Value, error) {
		var reason vm.Value = vm.Undefined
		if len(args) > 0 {
			reason = args[0]
		}
		return state.abort(reason), nil
	}))

	obj.SetOwnNonEnumerable("releaseLock", vm.NewNativeFunction(0, false, "releaseLock", func(args []vm.Value) (vm.Value, error) {
		state.release()
		return vm.Undefined, nil
	}))

	return vm.NewValueFromPlainObject(obj)
}

// createWritableStreamControllerObject builds the controller object handed
// to a JS-authored underlying sink's start(controller)/write(chunk,
// controller) hooks.
func createWritableStreamControllerObject(vmInstance *vm.VM, state *writableStreamState) vm.Value {
	obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	obj.SetOwnNonEnumerable("error", vm.NewNativeFunction(1, false, "error", func(args []vm.Value) (vm.Value, error) {
		var reason vm.Value = vm.Undefined
		if len(args) > 0 {
			reason = args[0]
		}
		state.errorOut(reason)
		return vm.Undefined, nil
	}))

	return vm.NewValueFromPlainObject(obj)
}
