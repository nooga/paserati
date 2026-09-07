package driver

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/nooga/paserati/pkg/builtins"
)

// TestFetchTransportDefaults is a guard against re-hardcoding the fetch
// transport's configuration surface (#290): fetch()'s http.Transport used
// to be built fresh per-request with Proxy left nil (so HTTP_PROXY/
// HTTPS_PROXY were silently ignored) and ResponseHeaderTimeout hardcoded to
// 30s with no way for a host to change it. builtins.FetchProxy/
// FetchResponseHeaderTimeout are the fix's configuration seam; this just
// checks their defaults didn't drift back to the broken shape.
func TestFetchTransportDefaults(t *testing.T) {
	if builtins.FetchProxy == nil {
		t.Fatal("builtins.FetchProxy is nil by default; HTTP_PROXY/HTTPS_PROXY would be silently ignored (#290)")
	}
	if builtins.FetchResponseHeaderTimeout != 30*time.Second {
		t.Fatalf("builtins.FetchResponseHeaderTimeout default = %v, want 30s", builtins.FetchResponseHeaderTimeout)
	}
}

// TestFetchRespectsConfiguredProxy is the behavioral regression test for
// #290's Proxy half: it points builtins.FetchProxy at a fixed httptest
// server (standing in for what http.ProxyFromEnvironment would resolve
// HTTP_PROXY to) and asserts a fetch() to a wholly different, unreachable
// URL actually lands on the proxy - i.e. the request was routed through
// the configured proxy rather than connecting directly (which would fail,
// since the target host doesn't exist). Deliberately does not exercise
// this via the HTTP_PROXY env var itself: http.ProxyFromEnvironment caches
// its answer per-process behind a sync.Once, so a test-scoped env var
// change can silently no-op depending on process history - testing the
// FetchProxy seam directly is the reliable seam to assert against.
func TestFetchRespectsConfiguredProxy(t *testing.T) {
	var gotRequestURI string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.RequestURI
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("failed to parse proxy URL: %v", err)
	}

	prevProxy := builtins.FetchProxy
	builtins.FetchProxy = func(*http.Request) (*url.URL, error) { return proxyURL, nil }
	defer func() { builtins.FetchProxy = prevProxy }()

	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	// This host does not exist; if the proxy weren't honored, the request
	// would fail with a DNS/connection error instead of resolving ok.
	script := `
		async function run() {
			const resp = await fetch("http://fetch-290-does-not-exist.invalid/some/path");
			return { ok: resp.ok, status: resp.status };
		}
		await run();
	`

	resultVal, errs := p.RunCode(script, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("script failed: %v (proxy not honored?)", errs[0])
	}
	result := resultVal.AsPlainObject()
	okVal, _ := result.GetOwn("ok")
	if !okVal.AsBoolean() {
		t.Fatal("fetch() did not resolve ok; request was not routed through the configured proxy")
	}

	if gotRequestURI == "" {
		t.Fatal("proxy never received a request; fetch() connected directly instead of using builtins.FetchProxy")
	}
}
