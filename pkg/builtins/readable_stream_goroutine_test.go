package builtins

import (
	"testing"
	"time"

	"github.com/nooga/paserati/pkg/vm"
)

// TestReadableStreamGoroutineFed proves paserati's Promise/VM layer can
// safely settle a ReadableStream read() from a background goroutine - the
// fork question #205's design turned on ("can this VM resolve a Promise
// from a Go goroutine?"). It deliberately mirrors fetch()'s own existing
// goroutine-resolves-a-promise pattern (doFetchRequestWithContext in
// fetch_init.go calls vm.ResolvePromise from its own goroutine with no
// extra synchronization at that call site) rather than introducing a new
// one, and exercises ReadableStreamController - the primitive a host
// integration (e.g. noderati's own incrementally-streamed fetch(), per
// #205's scope) would drive from its own HTTP-reading goroutine.
func TestReadableStreamGoroutineFed(t *testing.T) {
	vmInstance := vm.NewVM()

	// createReadableStreamObject wires up [Symbol.asyncIterator], which
	// normally comes from SymbolInitializer running first (see standard.go's
	// priority ordering); set it directly here rather than bootstrapping the
	// rest of that initializer's Symbol.prototype setup, which needs more of
	// the VM realm than this narrow test cares about.
	SymbolAsyncIterator = vm.NewSymbol("Symbol.asyncIterator")

	initializer := &ReadableStreamInitializer{}
	ctx := &RuntimeContext{
		VM:           vmInstance,
		DefineGlobal: func(name string, value vm.Value) error { return nil },
	}
	if err := initializer.InitRuntime(ctx); err != nil {
		t.Fatalf("InitRuntime failed: %v", err)
	}

	streamVal, controller := NewHostFedReadableStream(vmInstance)

	getReaderFn, ok := streamVal.AsPlainObject().GetOwn("getReader")
	if !ok || !getReaderFn.IsCallable() {
		t.Fatal("stream has no callable getReader")
	}
	readerVal, err := vmInstance.Call(getReaderFn, streamVal, nil)
	if err != nil {
		t.Fatalf("getReader() failed: %v", err)
	}
	readFn, ok := readerVal.AsPlainObject().GetOwn("read")
	if !ok || !readFn.IsCallable() {
		t.Fatal("reader has no callable read")
	}

	rt := vmInstance.GetAsyncRuntime()

	// Background goroutine, deliberately separate from the one driving the
	// VM below - the same shape fetch()'s own request goroutine takes,
	// bracketed with BeginExternalOp/EndExternalOp so DrainUntilIdle-style
	// waiting (reproduced manually below) knows to wait for it.
	rt.BeginExternalOp()
	go func() {
		time.Sleep(20 * time.Millisecond)
		controller.EnqueueBytes([]byte("hello"))
		controller.Close()
		rt.EndExternalOp()
	}()

	// read() itself must run on the "main" goroutine (it's always reached via
	// a native-function call from JS bytecode), same as any real reader.read().
	promiseVal, err := vmInstance.Call(readFn, readerVal, nil)
	if err != nil {
		t.Fatalf("read() failed: %v", err)
	}
	promise := promiseVal.AsPromise()
	if promise == nil {
		t.Fatal("read() did not return a Promise")
	}

	waitForSettled(t, rt, promise)

	if promise.State != vm.PromiseFulfilled {
		t.Fatalf("expected promise to be fulfilled, got state=%v result=%v", promise.State, promise.Result)
	}

	resultObj := promise.Result.AsPlainObject()
	if resultObj == nil {
		t.Fatalf("resolved value is not a plain object: %+v", promise.Result)
	}
	if doneVal, _ := resultObj.GetOwn("done"); doneVal.Type() != vm.TypeBoolean || doneVal.AsBoolean() {
		t.Fatalf("expected done=false on first chunk, got %v", doneVal)
	}
	valueVal, _ := resultObj.GetOwn("value")
	if valueVal.Type() != vm.TypeTypedArray {
		t.Fatalf("expected a Uint8Array chunk, got type %v", valueVal.Type())
	}
	ta := valueVal.AsTypedArray()
	got := make([]byte, ta.GetLength())
	for i := range got {
		got[i] = byte(ta.GetElement(i).ToFloat())
	}
	if string(got) != "hello" {
		t.Fatalf("expected chunk %q, got %q", "hello", string(got))
	}

	// The goroutine also called Close() right after the one chunk, so a
	// second read() must resolve (immediately, no further waiting needed)
	// with done: true.
	promiseVal2, err := vmInstance.Call(readFn, readerVal, nil)
	if err != nil {
		t.Fatalf("second read() failed: %v", err)
	}
	promise2 := promiseVal2.AsPromise()
	if promise2 == nil || promise2.State != vm.PromiseFulfilled {
		t.Fatalf("expected second read() to be immediately fulfilled, got %+v", promise2)
	}
	resultObj2 := promise2.Result.AsPlainObject()
	if doneVal2, _ := resultObj2.GetOwn("done"); doneVal2.Type() != vm.TypeBoolean || !doneVal2.AsBoolean() {
		t.Fatalf("expected done=true after close, got %v", doneVal2)
	}
}

// waitForSettled waits for promise to settle without ever reading its State
// before establishing a happens-before edge with whatever goroutine might
// still be resolving it: WaitForExternalOp/EndExternalOp share the runtime's
// own mutex (pkg/runtime/async.go), so a State read taken right after
// WaitForExternalOp returns is guaranteed to see every mutation the
// external-op goroutine made before its matching EndExternalOp call. This
// matters because plain "read promise.State" *before ever synchronizing at
// all* - even in a spin/poll loop - is a genuine, reproducible data race
// with a concurrent ResolvePromise/RejectPromise call (confirmed independent
// of ReadableStream: `go test -race` flags a bare NewPendingPromise() +
// goroutine-calls-ResolvePromise() + polling-read repro with no
// ReadableStream code involved at all). That gap also lives in the VM's own
// `await` opcode (pkg/vm/vm.go, `switch awaitedPromise.State` reads it with
// no synchronization whatsoever, not even this runtime's mutex) - out of
// scope to fix here, flagged separately.
func waitForSettled(t *testing.T, rt interface {
	HasPendingExternalOps() bool
	WaitForExternalOp()
}, promise *vm.PromiseObject) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if rt.HasPendingExternalOps() {
			rt.WaitForExternalOp()
		}
		if promise.State != vm.PromisePending {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for goroutine-fed read() to settle")
		}
	}
}

// drainUntilSettled runs the VM's real event-loop drain (nextTicks,
// microtasks, timers, external ops - see vm.DrainUntilIdle) until it goes
// idle, then asserts every given promise has settled. Needed (instead of
// waitForSettled's plain State-poll) whenever a promise's settlement depends
// on a *registered reaction* actually running rather than just on
// trySettle() being called directly - triggerPromiseReactions always
// defers reaction dispatch via ScheduleMicrotask, so something has to drain
// the microtask queue for that dispatch to happen at all.
func drainUntilSettled(t *testing.T, vmInstance *vm.VM, promises ...*vm.PromiseObject) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		vmInstance.DrainUntilIdle()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out draining the event loop")
	}
	for i, p := range promises {
		if p.State == vm.PromisePending {
			t.Fatalf("promise %d still pending after DrainUntilIdle", i)
		}
	}
}

// TestReadableStreamTeeGoroutineFed exercises tee() (#390) against the exact
// scenario the issue reports it for: a stream fed from a background
// goroutine (fetch's own streaming Response.body shape - NewHostFedReadableStream
// is the same primitive fetch_init.go builds on), not just a synchronous
// JS start() source. tee()'s own bookkeeping (reading/canceled1/canceled2 in
// its closure, see readable_stream_init.go) is documented as touched only
// from the main VM goroutine; this test - run with `-race` - is what backs
// that claim: enqueue/close happen on a separate goroutine same as
// TestReadableStreamGoroutineFed above, and both branches' read() calls
// (which is where tee's pull/reaction closures actually run) happen on this
// goroutine, same as any real reader.read().
func TestReadableStreamTeeGoroutineFed(t *testing.T) {
	vmInstance := vm.NewVM()
	SymbolAsyncIterator = vm.NewSymbol("Symbol.asyncIterator")

	initializer := &ReadableStreamInitializer{}
	ctx := &RuntimeContext{
		VM:           vmInstance,
		DefineGlobal: func(name string, value vm.Value) error { return nil },
	}
	if err := initializer.InitRuntime(ctx); err != nil {
		t.Fatalf("InitRuntime failed: %v", err)
	}

	streamVal, controller := NewHostFedReadableStream(vmInstance)

	teeFn, ok := streamVal.AsPlainObject().GetOwn("tee")
	if !ok || !teeFn.IsCallable() {
		t.Fatal("stream has no callable tee")
	}
	teeResult, err := vmInstance.Call(teeFn, streamVal, nil)
	if err != nil {
		t.Fatalf("tee() failed: %v", err)
	}
	branches := teeResult.AsArray()
	if branches == nil || branches.Length() != 2 {
		t.Fatalf("expected tee() to return a 2-element array, got %+v", teeResult)
	}

	getReader := func(streamV vm.Value) vm.Value {
		t.Helper()
		fn, ok := streamV.AsPlainObject().GetOwn("getReader")
		if !ok || !fn.IsCallable() {
			t.Fatal("branch has no callable getReader")
		}
		readerVal, err := vmInstance.Call(fn, streamV, nil)
		if err != nil {
			t.Fatalf("getReader() failed: %v", err)
		}
		return readerVal
	}
	readerA := getReader(branches.Get(0))
	readerB := getReader(branches.Get(1))

	rt := vmInstance.GetAsyncRuntime()

	rt.BeginExternalOp()
	go func() {
		time.Sleep(20 * time.Millisecond)
		controller.EnqueueBytes([]byte("hello"))
		controller.Close()
		rt.EndExternalOp()
	}()

	readOnce := func(readerVal vm.Value) *vm.PromiseObject {
		t.Helper()
		fn, ok := readerVal.AsPlainObject().GetOwn("read")
		if !ok || !fn.IsCallable() {
			t.Fatal("reader has no callable read")
		}
		promiseVal, err := vmInstance.Call(fn, readerVal, nil)
		if err != nil {
			t.Fatalf("read() failed: %v", err)
		}
		promise := promiseVal.AsPromise()
		if promise == nil {
			t.Fatal("read() did not return a Promise")
		}
		return promise
	}

	promiseA := readOnce(readerA)
	promiseB := readOnce(readerB)

	// Unlike a plain (non-teed) read(), whose promise trySettle()s directly
	// inside enqueue() (see TestReadableStreamGoroutineFed's waitForSettled,
	// which only ever needs to observe that), a tee() branch's read()
	// promise is only settled once the fulfillment reaction pullAlgorithm
	// registered on the ORIGINAL stream's own read() actually runs - and
	// triggerPromiseReactions always dispatches reactions via
	// ScheduleMicrotask (pkg/vm/promise.go), never synchronously. So this
	// needs a real event-loop drain (DrainUntilIdle: nextTicks, microtasks,
	// timers, then wait-for-external-op, repeat) - exactly what a real
	// script's `await` does - not just a State poll gated on external-op
	// completion.
	drainUntilSettled(t, vmInstance, promiseA, promiseB)

	checkChunk := func(label string, promise *vm.PromiseObject) {
		t.Helper()
		if promise.State != vm.PromiseFulfilled {
			t.Fatalf("%s: expected fulfilled, got state=%v result=%v", label, promise.State, promise.Result)
		}
		resultObj := promise.Result.AsPlainObject()
		if resultObj == nil {
			t.Fatalf("%s: resolved value is not a plain object: %+v", label, promise.Result)
		}
		if doneVal, _ := resultObj.GetOwn("done"); doneVal.Type() != vm.TypeBoolean || doneVal.AsBoolean() {
			t.Fatalf("%s: expected done=false on first chunk, got %v", label, doneVal)
		}
		valueVal, _ := resultObj.GetOwn("value")
		if valueVal.Type() != vm.TypeTypedArray {
			t.Fatalf("%s: expected a Uint8Array chunk, got type %v", label, valueVal.Type())
		}
		ta := valueVal.AsTypedArray()
		got := make([]byte, ta.GetLength())
		for i := range got {
			got[i] = byte(ta.GetElement(i).ToFloat())
		}
		if string(got) != "hello" {
			t.Fatalf("%s: expected chunk %q, got %q", label, "hello", string(got))
		}
	}
	checkChunk("branch A", promiseA)
	checkChunk("branch B", promiseB)

	// Both branches must independently see the close too.
	promiseA2 := readOnce(readerA)
	promiseB2 := readOnce(readerB)
	drainUntilSettled(t, vmInstance, promiseA2, promiseB2)
	for label, p := range map[string]*vm.PromiseObject{"branch A": promiseA2, "branch B": promiseB2} {
		if p.State != vm.PromiseFulfilled {
			t.Fatalf("%s: expected fulfilled after close, got state=%v", label, p.State)
		}
		resultObj := p.Result.AsPlainObject()
		if doneVal, _ := resultObj.GetOwn("done"); doneVal.Type() != vm.TypeBoolean || !doneVal.AsBoolean() {
			t.Fatalf("%s: expected done=true after close, got %v", label, doneVal)
		}
	}
}
