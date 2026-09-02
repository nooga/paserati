package builtins

import (
	"sync"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// PriorityReadableStream places ReadableStream before Blob (190), so a future
// Blob.stream() (currently a stub in blob_init.go) can build on this
// primitive without a reordering.
const PriorityReadableStream = 185

// ReadableStreamInitializer implements a minimal ReadableStream /
// ReadableStreamDefaultReader, sized to the surface real streaming
// consumers (the OpenAI/Anthropic/Google SDKs' shared
// ReadableStreamToAsyncIterable-style helper) actually need (#205):
//
//	stream[Symbol.asyncIterator]   // preferred if present, OR:
//	stream.getReader() -> {
//	  read(): Promise<{ done, value }>,
//	  releaseLock(): void,
//	  cancel(): Promise<void>,
//	}
//
// Not the full Web Streams API - no pipeThrough/tee backpressure semantics,
// no WritableStream, no BYOB readers. Per #205's scope, real incremental
// network streaming (feeding this from an HTTP response body as bytes
// arrive) is noderati's responsibility; this file's job is the primitive
// itself, plus a Go-facing feeding API (ReadableStreamController /
// NewHostFedReadableStream below) proven safe to drive from a background
// goroutine - see that type's doc comment for why.
type ReadableStreamInitializer struct{}

func (r *ReadableStreamInitializer) Name() string  { return "ReadableStream" }
func (r *ReadableStreamInitializer) Priority() int { return PriorityReadableStream }

func (r *ReadableStreamInitializer) InitTypes(ctx *TypeContext) error {
	readerType := types.NewObjectType().
		WithProperty("read", types.NewSimpleFunction([]types.Type{}, types.Any)). // Promise<{value, done}>
		WithProperty("releaseLock", types.NewSimpleFunction([]types.Type{}, types.Undefined)).
		WithProperty("cancel", types.NewOptionalFunction([]types.Type{types.Any}, types.Any, []bool{true}))

	streamType := types.NewObjectType().
		WithProperty("locked", types.Boolean).
		WithProperty("getReader", types.NewSimpleFunction([]types.Type{}, readerType)).
		WithProperty("cancel", types.NewOptionalFunction([]types.Type{types.Any}, types.Any, []bool{true}))

	streamCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, streamType).             // new ReadableStream()
		WithSimpleCallSignature([]types.Type{types.Any}, streamType).    // new ReadableStream(underlyingSource)
		WithProperty("prototype", streamType)

	return ctx.DefineGlobal("ReadableStream", streamCtorType)
}

func (r *ReadableStreamInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	streamProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	readerProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Stashed so NewHostFedReadableStream (a Go-facing constructor with no
	// JS underlyingSource) can build stream/reader objects that share the
	// same prototypes as ones constructed from script.
	readableStreamProto = streamProto
	readableStreamReaderProto = readerProto

	ctorFn := func(args []vm.Value) (vm.Value, error) {
		state := newReadableStreamState(vmInstance)
		streamVal := createReadableStreamObject(vmInstance, state, streamProto, readerProto)

		if len(args) > 0 && (args[0].Type() == vm.TypeObject || args[0].Type() == vm.TypeDictObject) {
			state.underlyingSource = args[0]
			if startFn, err := vmInstance.GetProperty(args[0], "start"); err == nil && startFn.IsCallable() {
				controllerVal := createReadableStreamControllerObject(vmInstance, state)
				if _, callErr := vmInstance.Call(startFn, args[0], []vm.Value{controllerVal}); callErr != nil {
					return vm.Undefined, callErr
				}
			}
		}

		return streamVal, nil
	}

	ctor := vm.NewConstructorWithProps(1, false, "ReadableStream", ctorFn)
	if ctor.Type() == vm.TypeNativeFunctionWithProps {
		ctor.AsNativeFunctionWithProps().Properties.DefineFixedProperty("prototype", vm.NewValueFromPlainObject(streamProto))
	}
	streamProto.SetOwnNonEnumerable("constructor", ctor)

	return ctx.DefineGlobal("ReadableStream", ctor)
}

// Package-level prototypes, set once during InitRuntime - mirrors the
// SymbolIterator/SymbolAsyncIterator package-var convention in
// symbol_init.go. Only valid after ReadableStreamInitializer.InitRuntime has
// run (i.e. after normal Paserati/driver initialization).
var (
	readableStreamProto       *vm.PlainObject
	readableStreamReaderProto *vm.PlainObject
)

// pendingStreamRead is a read() call that arrived while the queue was empty
// and the stream neither closed nor errored; it is settled the moment a
// chunk is enqueued, the stream closes, or the stream errors.
type pendingStreamRead struct {
	promise *vm.PromiseObject
}

// readableStreamState is the internal, non-JS-visible state backing a
// ReadableStream. All mutation goes through its methods, which take `mu` -
// this is what makes enqueue/close/errorOut safe to call from a goroutine
// that isn't the VM's own execution goroutine (see ReadableStreamController).
type readableStreamState struct {
	mu sync.Mutex

	vmInstance *vm.VM
	streamObj  *vm.PlainObject // for keeping the JS-visible `locked` property in sync

	queue        []vm.Value
	pendingReads []*pendingStreamRead

	closed     bool
	errored    bool
	errorValue vm.Value

	locked bool

	underlyingSource vm.Value // Undefined if none was given
}

func newReadableStreamState(vmInstance *vm.VM) *readableStreamState {
	return &readableStreamState{vmInstance: vmInstance, underlyingSource: vm.Undefined}
}

// iterResultValue builds a plain {value, done} object, the shape read()/
// next() promises resolve to. Safe to call from any goroutine: it only
// allocates a brand-new, not-yet-shared PlainObject (see NewObject/SetOwn's
// own internal Shape.mu locking in pkg/vm/object.go for why concurrent
// object creation is already relied upon - fetch()'s response-building
// goroutine does the same).
func iterResultValue(vmInstance *vm.VM, value vm.Value, done bool) vm.Value {
	obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	obj.SetOwnNonEnumerable("value", value)
	obj.SetOwnNonEnumerable("done", vm.BooleanValue(done))
	return vm.NewValueFromPlainObject(obj)
}

// errorValueFromGoError unwraps a Go error from vm.Call/NewTypeError etc.
// into the actual thrown Value, mirroring the identical unwrap in
// pkg/vm/promise.go's triggerPromiseReactions (a plain NewString(err.Error())
// would silently replace a real Error object with a message-less string).
func errorValueFromGoError(err error) vm.Value {
	if err == nil {
		return vm.Undefined
	}
	if ee, ok := err.(vm.ExceptionError); ok {
		return ee.GetExceptionValue()
	}
	return vm.NewString(err.Error())
}

// enqueue makes a chunk available to the stream. Safe to call from any
// goroutine - see the type doc comment on readableStreamState/
// ReadableStreamController.
func (s *readableStreamState) enqueue(chunk vm.Value) {
	s.mu.Lock()
	if s.closed || s.errored {
		s.mu.Unlock()
		return
	}
	if len(s.pendingReads) > 0 {
		pr := s.pendingReads[0]
		s.pendingReads = s.pendingReads[1:]
		s.mu.Unlock()
		s.vmInstance.ResolvePromise(pr.promise, iterResultValue(s.vmInstance, chunk, false))
		return
	}
	s.queue = append(s.queue, chunk)
	s.mu.Unlock()
}

// close signals a normal end of stream. Safe to call from any goroutine.
func (s *readableStreamState) close() {
	s.mu.Lock()
	if s.closed || s.errored {
		s.mu.Unlock()
		return
	}
	s.closed = true
	pending := s.pendingReads
	s.pendingReads = nil
	s.mu.Unlock()

	for _, pr := range pending {
		s.vmInstance.ResolvePromise(pr.promise, iterResultValue(s.vmInstance, vm.Undefined, true))
	}
}

// errorOut signals an abnormal end of stream, rejecting any read() already
// waiting and every one still to come. Safe to call from any goroutine.
func (s *readableStreamState) errorOut(reason vm.Value) {
	s.mu.Lock()
	if s.closed || s.errored {
		s.mu.Unlock()
		return
	}
	s.errored = true
	s.errorValue = reason
	pending := s.pendingReads
	s.pendingReads = nil
	s.mu.Unlock()

	for _, pr := range pending {
		s.vmInstance.RejectPromise(pr.promise, reason)
	}
}

// read implements reader.read()/the async-iterator's next(). Unlike
// enqueue/close/errorOut, this must only be called from the main VM
// goroutine - it's always reached via a native-function call from JS, and it
// may itself synchronously call back into JS (the underlying source's
// pull() hook).
func (s *readableStreamState) read() vm.Value {
	s.mu.Lock()
	if len(s.queue) > 0 {
		chunk := s.queue[0]
		s.queue = s.queue[1:]
		s.mu.Unlock()
		return s.vmInstance.NewResolvedPromise(iterResultValue(s.vmInstance, chunk, false))
	}
	if s.errored {
		reason := s.errorValue
		s.mu.Unlock()
		return s.vmInstance.NewRejectedPromise(reason)
	}
	if s.closed {
		s.mu.Unlock()
		return s.vmInstance.NewResolvedPromise(iterResultValue(s.vmInstance, vm.Undefined, true))
	}

	promiseVal := s.vmInstance.NewPendingPromise()
	s.pendingReads = append(s.pendingReads, &pendingStreamRead{promise: promiseVal.AsPromise()})
	source := s.underlyingSource
	s.mu.Unlock()

	// Best-effort backpressure signal: ask a JS-authored underlying source
	// for more data now that a consumer is waiting on an empty queue. Not
	// spec-accurate (no highWaterMark bookkeeping), but enough to drive a
	// simple pull-based source.
	if source.Type() == vm.TypeObject || source.Type() == vm.TypeDictObject {
		if pullFn, err := s.vmInstance.GetProperty(source, "pull"); err == nil && pullFn.IsCallable() {
			controllerVal := createReadableStreamControllerObject(s.vmInstance, s)
			_, _ = s.vmInstance.Call(pullFn, source, []vm.Value{controllerVal})
		}
	}

	return promiseVal
}

// cancel implements reader.cancel()/stream.cancel(): it stops the stream
// (as if closed) and forwards to the underlying source's cancel() hook, if
// any.
func (s *readableStreamState) cancel(reason vm.Value) vm.Value {
	s.mu.Lock()
	if s.closed || s.errored {
		s.mu.Unlock()
		return s.vmInstance.NewResolvedPromise(vm.Undefined)
	}
	s.closed = true
	// Per spec, cancelling resets the queue - a cancelled stream must not
	// keep handing out chunks it had already buffered before the cancel.
	s.queue = nil
	pending := s.pendingReads
	s.pendingReads = nil
	source := s.underlyingSource
	s.mu.Unlock()

	for _, pr := range pending {
		s.vmInstance.ResolvePromise(pr.promise, iterResultValue(s.vmInstance, vm.Undefined, true))
	}

	if source.Type() == vm.TypeObject || source.Type() == vm.TypeDictObject {
		if cancelFn, err := s.vmInstance.GetProperty(source, "cancel"); err == nil && cancelFn.IsCallable() {
			result, callErr := s.vmInstance.Call(cancelFn, source, []vm.Value{reason})
			if callErr != nil {
				return s.vmInstance.NewRejectedPromise(errorValueFromGoError(callErr))
			}
			return s.vmInstance.NewResolvedPromise(result)
		}
	}
	return s.vmInstance.NewResolvedPromise(vm.Undefined)
}

// release implements reader.releaseLock(), unlocking the stream so a new
// reader can be acquired via getReader().
func (s *readableStreamState) release() {
	s.mu.Lock()
	s.locked = false
	obj := s.streamObj
	s.mu.Unlock()
	if obj != nil {
		obj.SetOwn("locked", vm.False)
	}
}

func createReadableStreamObject(vmInstance *vm.VM, state *readableStreamState, streamProto, readerProto *vm.PlainObject) vm.Value {
	obj := vm.NewObject(vm.NewValueFromPlainObject(streamProto)).AsPlainObject()
	state.streamObj = obj
	obj.SetOwn("locked", vm.False)

	obj.SetOwnNonEnumerable("getReader", vm.NewNativeFunction(0, false, "getReader", func(args []vm.Value) (vm.Value, error) {
		state.mu.Lock()
		if state.locked {
			state.mu.Unlock()
			return vm.Undefined, vmInstance.NewTypeError("ReadableStream is already locked to a reader")
		}
		state.locked = true
		state.mu.Unlock()
		obj.SetOwn("locked", vm.True)
		return createReadableStreamReaderObject(vmInstance, state, readerProto), nil
	}))

	obj.SetOwnNonEnumerable("cancel", vm.NewNativeFunction(1, false, "cancel", func(args []vm.Value) (vm.Value, error) {
		var reason vm.Value = vm.Undefined
		if len(args) > 0 {
			reason = args[0]
		}
		return state.cancel(reason), nil
	}))

	// [Symbol.asyncIterator] - the surface every real SDK's own
	// ReadableStreamToAsyncIterable shim tries first (#205's scope note).
	asyncIterFn := vm.NewNativeFunction(0, false, "[Symbol.asyncIterator]", func(args []vm.Value) (vm.Value, error) {
		return createReadableStreamAsyncIteratorObject(vmInstance, state), nil
	})
	w, e, c := true, false, true
	obj.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolAsyncIterator), asyncIterFn, &w, &e, &c)

	return vm.NewValueFromPlainObject(obj)
}

func createReadableStreamReaderObject(vmInstance *vm.VM, state *readableStreamState, readerProto *vm.PlainObject) vm.Value {
	obj := vm.NewObject(vm.NewValueFromPlainObject(readerProto)).AsPlainObject()
	released := false

	obj.SetOwnNonEnumerable("read", vm.NewNativeFunction(0, false, "read", func(args []vm.Value) (vm.Value, error) {
		if released {
			err := vmInstance.NewTypeError("Cannot read a stream using a released reader")
			return vmInstance.NewRejectedPromise(errorValueFromGoError(err)), nil
		}
		return state.read(), nil
	}))

	obj.SetOwnNonEnumerable("releaseLock", vm.NewNativeFunction(0, false, "releaseLock", func(args []vm.Value) (vm.Value, error) {
		released = true
		state.release()
		return vm.Undefined, nil
	}))

	obj.SetOwnNonEnumerable("cancel", vm.NewNativeFunction(1, false, "cancel", func(args []vm.Value) (vm.Value, error) {
		var reason vm.Value = vm.Undefined
		if len(args) > 0 {
			reason = args[0]
		}
		return state.cancel(reason), nil
	}))

	return vm.NewValueFromPlainObject(obj)
}

// createReadableStreamControllerObject builds the controller object handed
// to a JS-authored underlying source's start(controller)/pull(controller)
// hooks (enqueue/close/error), per the standard ReadableStreamController
// shape.
func createReadableStreamControllerObject(vmInstance *vm.VM, state *readableStreamState) vm.Value {
	obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	obj.SetOwnNonEnumerable("enqueue", vm.NewNativeFunction(1, false, "enqueue", func(args []vm.Value) (vm.Value, error) {
		var chunk vm.Value = vm.Undefined
		if len(args) > 0 {
			chunk = args[0]
		}
		state.enqueue(chunk)
		return vm.Undefined, nil
	}))

	obj.SetOwnNonEnumerable("close", vm.NewNativeFunction(0, false, "close", func(args []vm.Value) (vm.Value, error) {
		state.close()
		return vm.Undefined, nil
	}))

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

// createReadableStreamAsyncIteratorObject implements the object returned by
// stream[Symbol.asyncIterator](), consuming the stream directly (it does not
// go through getReader()'s single-lock bookkeeping - matching real
// browsers/Node, where for-await-of a stream doesn't observe `locked`).
func createReadableStreamAsyncIteratorObject(vmInstance *vm.VM, state *readableStreamState) vm.Value {
	obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	obj.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		return state.read(), nil
	}))

	obj.SetOwnNonEnumerable("return", vm.NewNativeFunction(1, false, "return", func(args []vm.Value) (vm.Value, error) {
		var val vm.Value = vm.Undefined
		if len(args) > 0 {
			val = args[0]
		}
		state.cancel(vm.Undefined)
		return iterResultValue(vmInstance, val, true), nil
	}))

	selfFn := vm.NewNativeFunction(0, false, "[Symbol.asyncIterator]", func(args []vm.Value) (vm.Value, error) {
		return vm.NewValueFromPlainObject(obj), nil
	})
	w, e, c := true, false, true
	obj.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolAsyncIterator), selfFn, &w, &e, &c)

	return vm.NewValueFromPlainObject(obj)
}

// ReadableStreamController is the host-facing (Go) handle for feeding a
// ReadableStream from outside JS - typically a background goroutine reading
// an HTTP response body incrementally (the piece #205 scopes to noderati's
// own streaming fetch(), not to paserati's built-in one). Every method is
// safe to call concurrently with the VM's own execution goroutine:
//
//   - It uses the exact promise-resolved-from-a-goroutine pattern fetch()
//     already relies on today: doFetchRequestWithContext (fetch_init.go)
//     calls vm.ResolvePromise/vm.RejectPromise from its own goroutine once
//     the HTTP round-trip completes, with no additional synchronization at
//     that call site.
//   - The object/value allocation those calls do under the hood (a fresh
//     {value, done} object per chunk) is itself safe from any goroutine:
//     PlainObject shape transitions are protected by their own Shape.mu
//     (pkg/vm/object.go's SetOwn), which is exactly the mechanism that lets
//     fetch()'s goroutine build its whole Response object off-thread before
//     handing it to ResolvePromise.
//   - This type's own state (readableStreamState) adds its own mutex around
//     the queue/pending-reads bookkeeping that promise resolution alone
//     doesn't cover.
//
// A host would typically call rt.BeginExternalOp()/EndExternalOp() (see
// vm.VM.GetAsyncRuntime) around the feeding goroutine's lifetime, the same
// way fetch() does, so DrainUntilIdle knows to wait for it.
type ReadableStreamController struct {
	state *readableStreamState
}

// Enqueue makes a chunk (typically a Uint8Array) available to the stream.
func (c *ReadableStreamController) Enqueue(chunk vm.Value) { c.state.enqueue(chunk) }

// EnqueueBytes wraps raw bytes as a Uint8Array chunk before enqueuing it -
// the representation real fetch()/HTTP consumers expect from a byte stream.
func (c *ReadableStreamController) EnqueueBytes(data []byte) {
	bufVal := vm.NewArrayBuffer(len(data))
	backing := bufVal.AsArrayBuffer()
	copy(backing.GetData(), data)
	c.state.enqueue(vm.NewTypedArray(vm.TypedArrayUint8, backing, 0, -1))
}

// Close signals a normal end of stream.
func (c *ReadableStreamController) Close() { c.state.close() }

// Error signals an abnormal end of stream, rejecting every pending and
// future read() with reason.
func (c *ReadableStreamController) Error(reason vm.Value) { c.state.errorOut(reason) }

// NewHostFedReadableStream creates a ReadableStream with no JS-authored
// underlying source, whose contents are instead fed entirely from Go via the
// returned ReadableStreamController. This is the primitive a real
// incrementally-streamed fetch() Response.body (left to noderati per #205)
// would be built on. Only valid once ReadableStreamInitializer.InitRuntime
// has already run against vmInstance (i.e. after normal
// Paserati/driver initialization).
func NewHostFedReadableStream(vmInstance *vm.VM) (vm.Value, *ReadableStreamController) {
	state := newReadableStreamState(vmInstance)
	streamVal := createReadableStreamObject(vmInstance, state, readableStreamProto, readableStreamReaderProto)
	return streamVal, &ReadableStreamController{state: state}
}
