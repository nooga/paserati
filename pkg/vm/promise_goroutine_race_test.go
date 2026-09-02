package vm

import (
	"testing"
	"time"
)

// TestPromiseGoroutineResolveRace is the minimal repro for the data race
// this fix closes: settling a Promise from a goroutine other than the one
// reading its state/result (exactly what fetch() does today via
// doFetchRequestWithContext calling vm.ResolvePromise from its own request
// goroutine) used to race unsynchronized against any direct read of
// PromiseObject.State/Result - most notably the `await` opcode's own
// `switch awaitedPromise.State` (vm.go), but reproducible with nothing more
// than this: no fetch(), no ReadableStream, no other feature involved.
// See pkg/driver/promise_goroutine_race_test.go for the same race exercised
// through the real `await` opcode end to end.
func TestPromiseGoroutineResolveRace(t *testing.T) {
	vmInstance := NewVM()
	p := vmInstance.NewPendingPromise()
	promise := p.AsPromise()

	go func() {
		time.Sleep(5 * time.Millisecond)
		vmInstance.ResolvePromise(promise, NumberValue(42))
	}()

	// GetState()/GetResult() are the thread-safe accessors PromiseObject's
	// mu backs - this is deliberately the exact same "poll from the calling
	// goroutine" shape the original bug report used, just going through the
	// now-locked methods instead of direct field access.
	deadline := time.Now().Add(2 * time.Second)
	for promise.GetState() == PromisePending {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for goroutine to resolve promise")
		}
		time.Sleep(time.Millisecond)
	}

	if state := promise.GetState(); state != PromiseFulfilled {
		t.Fatalf("expected PromiseFulfilled, got %v", state)
	}
	if result := promise.GetResult(); result.ToFloat() != 42 {
		t.Fatalf("expected 42, got %v", result.Inspect())
	}
}

// TestPromiseGoroutineRejectRace mirrors the above for rejection.
func TestPromiseGoroutineRejectRace(t *testing.T) {
	vmInstance := NewVM()
	p := vmInstance.NewPendingPromise()
	promise := p.AsPromise()

	go func() {
		time.Sleep(5 * time.Millisecond)
		vmInstance.RejectPromise(promise, NewString("boom"))
	}()

	deadline := time.Now().Add(2 * time.Second)
	for promise.GetState() == PromisePending {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for goroutine to reject promise")
		}
		time.Sleep(time.Millisecond)
	}

	if state := promise.GetState(); state != PromiseRejected {
		t.Fatalf("expected PromiseRejected, got %v", state)
	}
	if result := promise.GetResult(); result.ToString() != "boom" {
		t.Fatalf("expected %q, got %v", "boom", result.Inspect())
	}
}

// TestAddAwaitReactionsDoesNotLeakOnSettledPromise is a regression test for
// a leak the first version of this fix introduced: awaiting an
// already-settled promise repeatedly (e.g. `for (;;) { await
// cachedResolvedPromise }`, a perfectly ordinary pattern) used to append one
// dead reaction of the *opposite*, permanently-unreachable disposition per
// call - since a settled promise's disposition never changes, a fulfilled
// promise's RejectReactions (or a rejected promise's FulfillReactions) can
// never fire, but an earlier version of this fix appended one there anyway
// on every await, retaining its closure forever. addAwaitReactions must
// register only the reaction that can actually still fire once a promise is
// already settled - see its doc comment - so both lists must stay at
// exactly the one live reaction actually needed, not grow with each call.
func TestAddAwaitReactionsDoesNotLeakOnSettledPromise(t *testing.T) {
	noop := PromiseReaction{Resolve: func(Value) {}, Reject: func(Value) {}}

	t.Run("fulfilled", func(t *testing.T) {
		p := NewVM().NewPendingPromise().AsPromise()
		if !p.trySettle(PromiseFulfilled, Undefined) {
			t.Fatal("trySettle failed on a fresh pending promise")
		}
		for i := range 1000 {
			if state := p.addAwaitReactions(noop, noop); state != PromiseFulfilled {
				t.Fatalf("iteration %d: expected PromiseFulfilled, got %v", i, state)
			}
		}
		if got := len(p.FulfillReactions); got != 1000 {
			t.Errorf("FulfillReactions: expected 1000 (one per await, each still reachable via the immutable Fulfilled state), got %d", got)
		}
		if got := len(p.RejectReactions); got != 0 {
			t.Errorf("RejectReactions: expected 0 (a fulfilled promise can never reject, so none of these should have been registered), got %d", got)
		}
	})

	t.Run("rejected", func(t *testing.T) {
		p := NewVM().NewPendingPromise().AsPromise()
		if !p.trySettle(PromiseRejected, Undefined) {
			t.Fatal("trySettle failed on a fresh pending promise")
		}
		for i := range 1000 {
			if state := p.addAwaitReactions(noop, noop); state != PromiseRejected {
				t.Fatalf("iteration %d: expected PromiseRejected, got %v", i, state)
			}
		}
		if got := len(p.RejectReactions); got != 1000 {
			t.Errorf("RejectReactions: expected 1000 (one per await, each still reachable via the immutable Rejected state), got %d", got)
		}
		if got := len(p.FulfillReactions); got != 0 {
			t.Errorf("FulfillReactions: expected 0 (a rejected promise can never fulfill, so none of these should have been registered), got %d", got)
		}
	})

	t.Run("pending registers both", func(t *testing.T) {
		p := NewVM().NewPendingPromise().AsPromise()
		if state := p.addAwaitReactions(noop, noop); state != PromisePending {
			t.Fatalf("expected PromisePending, got %v", state)
		}
		if got := len(p.FulfillReactions); got != 1 {
			t.Errorf("FulfillReactions: expected 1 (disposition still unknown, either could fire), got %d", got)
		}
		if got := len(p.RejectReactions); got != 1 {
			t.Errorf("RejectReactions: expected 1 (disposition still unknown, either could fire), got %d", got)
		}
	})
}
