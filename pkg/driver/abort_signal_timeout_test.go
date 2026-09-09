package driver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFetchAbortSignalTimeoutFires is the end-to-end regression test for
// #374: AbortSignal.timeout(ms) used to be a stub that never actually
// aborted anything. It's now wired to an *unref'd* timer
// (runtime.AsyncRuntime.ScheduleUnrefTimer) rather than a plain one,
// specifically so it doesn't by itself keep a script's event loop alive -
// but paired with an in-flight fetch() (which does keep the loop alive via
// BeginExternalOp/EndExternalOp), it must still fire and cancel the
// request once the deadline passes.
//
// The server never responds (blocks on `unblock`), so the only way this
// fetch() can settle within the test is via the timeout firing.
func TestFetchAbortSignalTimeoutFires(t *testing.T) {
	unblock := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-unblock
	}))
	defer func() {
		close(unblock)
		server.Close()
	}()

	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	script := `
		async function run() {
			try {
				await fetch("` + server.URL + `", { signal: AbortSignal.timeout(20) });
				return { ok: true };
			} catch (e) {
				return {
					ok: false,
					name: e.name,
					isError: e instanceof Error,
					message: e.message,
				};
			}
		}
		await run();
	`

	resultVal, errs := p.RunCode(script, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("script failed: %v", errs[0])
	}
	if !resultVal.IsObject() {
		t.Fatalf("script result is not an object: %#v", resultVal)
	}
	result := resultVal.AsPlainObject()

	okVal, _ := result.GetOwn("ok")
	if okVal.AsBoolean() {
		t.Fatal("fetch() unexpectedly resolved; AbortSignal.timeout(20) should have cancelled it")
	}

	isErrorVal, _ := result.GetOwn("isError")
	if !isErrorVal.AsBoolean() {
		t.Fatal("rejection is not `instanceof Error`")
	}

	// fetch()'s own abort-classification always names its rejection
	// "AbortError" regardless of the triggering signal's actual reason
	// (pre-existing behavior, unrelated to #374) - so the reason's own
	// name ("TimeoutError") only survives in the message text. Checking
	// for it here confirms the *right* reason actually made it through
	// AbortSignal.timeout(), not just that abort() fired at all.
	messageVal, _ := result.GetOwn("message")
	if msg := messageVal.ToString(); !strings.Contains(msg, "TimeoutError") {
		t.Fatalf("rejection message = %q, want it to mention TimeoutError", msg)
	}
}
