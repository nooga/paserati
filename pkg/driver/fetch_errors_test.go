package driver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFetchPlainNetworkErrorIsNotAbortError is the discriminating regression
// test for #213: a plain network failure with no AbortSignal involved at
// all must reject with the real underlying error, not a false AbortError.
// The bug was that doFetchRequestWithContext's own cleanup unconditionally
// cancels the request's context on every early return, so by the time the
// caller checked ctx.Err() == context.Canceled it was always true - turning
// every non-abort failure into a bogus "AbortError: The operation was
// aborted", masking the real cause. It also doubled as a regression test
// for #214 (rejections must be real Error instances, not bare strings).
func TestFetchPlainNetworkErrorIsNotAbortError(t *testing.T) {
	// A server that's already closed gives a deterministic, hermetic
	// connection-refused error - no reliance on external DNS/network.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	script := `
		async function run() {
			try {
				await fetch("` + url + `");
				return { ok: true };
			} catch (e) {
				return {
					ok: false,
					name: e.name,
					isError: e instanceof Error,
					hasMessage: typeof e.message === "string" && e.message.length > 0,
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
		t.Fatal("fetch() unexpectedly resolved; expected a connection-refused rejection")
	}

	nameVal, _ := result.GetOwn("name")
	if got := nameVal.ToString(); got != "TypeError" {
		t.Fatalf("rejection name = %q, want %q (the regression turned this into \"AbortError\")", got, "TypeError")
	}

	isErrorVal, _ := result.GetOwn("isError")
	if !isErrorVal.AsBoolean() {
		t.Fatal("rejection is not `instanceof Error` - fetch() must reject with a real Error object, not a bare string (#214)")
	}

	hasMessageVal, _ := result.GetOwn("hasMessage")
	if !hasMessageVal.AsBoolean() {
		t.Fatal("rejection has no non-empty string .message")
	}
}

// TestFetchAbortIsStillAbortError is a companion to the above: an actual
// AbortSignal-driven cancellation must still reject as a real AbortError
// (name "AbortError", instanceof Error) after the #213 fix.
func TestFetchAbortIsStillAbortError(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	script := `
		async function run() {
			try {
				const controller = new AbortController();
				controller.abort("stop");
				await fetch("http://example.invalid/", { signal: controller.signal });
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
		t.Fatal("fetch() unexpectedly resolved; expected a pre-aborted rejection")
	}

	nameVal, _ := result.GetOwn("name")
	if got := nameVal.ToString(); got != "AbortError" {
		t.Fatalf("rejection name = %q, want %q", got, "AbortError")
	}

	isErrorVal, _ := result.GetOwn("isError")
	if !isErrorVal.AsBoolean() {
		t.Fatal("rejection is not `instanceof Error` (#214)")
	}

	messageVal, _ := result.GetOwn("message")
	if got := messageVal.ToString(); got != "stop" {
		t.Fatalf("rejection message = %q, want %q", got, "stop")
	}
}
