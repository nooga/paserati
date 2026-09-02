package driver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestFetchStreamsResponseBody is the discriminating regression test for
// #205: fetch()'s Response.body must be a real ReadableStream fed
// incrementally off the network connection, not a stream (or a plain
// buffer) that only appears once the whole response has already arrived.
//
// A server that writes its response and closes immediately would pass even
// with the promise still resolving only after a full io.ReadAll, or with a
// single pre-filled chunk - see doFetchRequestWithContext/createResponseObject
// in pkg/builtins/fetch_init.go. This one flushes a chunk, sleeps well past
// the flush interval, flushes a second chunk, then closes, and asserts the
// script actually observed two separate reads with a real gap between them.
func TestFetchStreamsResponseBody(t *testing.T) {
	const flushGap = 150 * time.Millisecond

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not support flushing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("chunk-one\n"))
		flusher.Flush()
		time.Sleep(flushGap)
		_, _ = w.Write([]byte("chunk-two\n"))
		flusher.Flush()
	}))
	defer server.Close()

	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	script := fmt.Sprintf(`
		globalThis.__events = [];
		async function run() {
			const t0 = Date.now();
			const resp = await fetch(%q);
			__events.push({ t: Date.now() - t0, kind: "headers" });
			const reader = resp.body.getReader();
			while (true) {
				const { done } = await reader.read();
				if (done) break;
				__events.push({ t: Date.now() - t0, kind: "chunk" });
			}
			__events.push({ t: Date.now() - t0, kind: "done" });
		}
		await run();
	`, server.URL)

	// RunCode drains until the VM is fully idle (DrainUntilIdle), which
	// includes fetch's now-longer-lived body goroutine - see
	// doFetchRequestWithContext's streaming/cancel/EndExternalOp handoff.
	if _, errs := p.RunCode(script, RunOptions{}); len(errs) > 0 {
		t.Fatalf("script failed: %v", errs[0])
	}

	eventsVal, ok := p.GetVM().GetGlobal("__events")
	if !ok || !eventsVal.IsArray() {
		t.Fatal("__events global missing or not an array")
	}
	events := eventsVal.AsArray()

	type event struct {
		t    float64
		kind string
	}
	var got []event
	for i := 0; i < events.Length(); i++ {
		obj := events.Get(i).AsPlainObject()
		tv, _ := obj.GetOwn("t")
		kv, _ := obj.GetOwn("kind")
		got = append(got, event{t: tv.ToFloat(), kind: kv.ToString()})
	}

	want := []string{"headers", "chunk", "chunk", "done"}
	if len(got) != len(want) {
		t.Fatalf("got %d events %+v, want %d events with kinds %v", len(got), got, len(want), want)
	}
	for i, w := range want {
		if got[i].kind != w {
			t.Fatalf("event %d: got kind %q, want %q (all events: %+v)", i, got[i].kind, w, got)
		}
	}

	// The discriminating assertion: headers must resolve well before the
	// server's second flush, and the second chunk must arrive only after a
	// real gap - not both chunks showing up back-to-back once the whole
	// response (and the connection close) has already happened.
	if got[0].t > float64(flushGap.Milliseconds())/2 {
		t.Fatalf("fetch() resolved at %vms, too late - looks like it waited for the full body instead of just headers: %+v", got[0].t, got)
	}
	gap := got[2].t - got[1].t
	if gap < float64(flushGap.Milliseconds())/2 {
		t.Fatalf("second chunk arrived only %vms after the first (want >= ~%v) - looks like both chunks were buffered and delivered together: %+v", gap, flushGap/2, got)
	}
}
