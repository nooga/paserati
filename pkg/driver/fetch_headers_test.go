package driver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFetchForwardsRealHeadersInstance is a regression test for #237: a
// real Headers object's data lives behind every accessor (.get/.set/
// .append/etc) installed non-enumerable via SetOwnNonEnumerable (see
// createHeadersObject in pkg/builtins/fetch_init.go), so Object.keys()/
// OwnKeys() on it is always empty. fetch()'s request builder used to read
// init.headers purely via OwnKeys(), which works for a plain object
// literal like { "X-Foo": "bar" } but silently drops every header set
// through the real Headers API - new Headers(), .set(), .append() - so
// fetch(url, { headers: new Headers() }) sent none of them, and the server
// saw only Go's own http.Client defaults.
func TestFetchForwardsRealHeadersInstance(t *testing.T) {
	var gotAuth, gotCustom string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCustom = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	script := `
		async function run() {
			const h = new Headers();
			h.set("Authorization", "Bearer secret123");
			h.append("X-Custom", "hello");
			await fetch("` + server.URL + `", { headers: h });
		}
		await run();
	`

	_, errs := p.RunCode(script, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("script failed: %v", errs[0])
	}

	if gotAuth != "Bearer secret123" {
		t.Fatalf("Authorization header = %q, want %q (a real Headers instance's data was dropped)", gotAuth, "Bearer secret123")
	}
	if gotCustom != "hello" {
		t.Fatalf("X-Custom header = %q, want %q (a real Headers instance's data was dropped)", gotCustom, "hello")
	}
}

// TestHeadersConstructorAcceptsExistingHeadersInstance covers the same
// OwnKeys()-blindness bug (#237) in the Headers constructor itself:
// new Headers(anotherHeadersInstance) must copy the source's entries, not
// silently produce an empty Headers object.
func TestHeadersConstructorAcceptsExistingHeadersInstance(t *testing.T) {
	p := NewPaserati()
	p.SetSkipTypeCheck(true)

	script := `
		const src = new Headers();
		src.set("X-Test", "abc");
		const copy = new Headers(src);
		copy.get("X-Test");
	`

	result, errs := p.RunCode(script, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("script failed: %v", errs[0])
	}
	if result.ToString() != "abc" {
		t.Fatalf("copy.get(\"X-Test\") = %q, want %q", result.ToString(), "abc")
	}
}
