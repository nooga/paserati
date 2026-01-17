package driver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGlobalFetch tests the basic fetch functionality with global async fetch
func TestGlobalFetch(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check request method
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Return a simple JSON response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "Hello from test server", "path": "` + r.URL.Path + `"}`))
	}))
	defer server.Close()

	// Create Paserati instance
	p := NewPaserati()

	// Test TypeScript code using global async fetch
	tsCode := fmt.Sprintf(`
		// No import needed - fetch is global
		console.log("Testing fetch...");

		async function runTest() {
			// Perform a GET request (async)
			const response = await fetch("%s/test");
			console.log("Response status:", response.status);
			console.log("Response OK:", response.ok);

			// Get response text (async)
			const text = await response.text();
			console.log("Response text:", text);

			// Parse JSON
			const data = JSON.parse(text);
			console.log("Parsed data:", data);

			// Return test result
			return data.message === "Hello from test server" ? "fetch_test_passed" : "fetch_test_failed";
		}

		await runTest();
	`, server.URL)

	result, errs := p.RunCode(tsCode, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("Failed to run fetch test: %v", errs[0])
	}

	if result.ToString() != "fetch_test_passed" {
		t.Errorf("Expected 'fetch_test_passed', got: %v", result.ToString())
	}
}

// TestGlobalHeaders tests the Headers class functionality
func TestGlobalHeaders(t *testing.T) {
	p := NewPaserati()

	// Headers is now a global constructor
	tsCode := `
		// No import needed - Headers is global
		console.log("Testing Headers class...");

		// Create headers
		const headers = new Headers({
			"Content-Type": "application/json",
			"Authorization": "Bearer token123"
		});

		// Test get method
		console.log("Content-Type:", headers.get("Content-Type"));
		console.log("Authorization:", headers.get("Authorization"));

		// Test has method
		console.log("Has Content-Type:", headers.has("Content-Type"));
		console.log("Has X-Custom:", headers.has("X-Custom"));

		// Test set method
		headers.set("X-Custom", "custom-value");
		console.log("X-Custom after set:", headers.get("X-Custom"));

		// Test delete method
		headers.delete("Authorization");
		console.log("Has Authorization after delete:", headers.has("Authorization"));

		// Return success
		"headers_test_passed";
	`

	result, errs := p.RunCode(tsCode, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("Failed to run headers test: %v", errs[0])
	}

	if result.ToString() != "headers_test_passed" {
		t.Errorf("Expected 'headers_test_passed', got: %v", result.ToString())
	}
}

// TestGlobalFetchPOST tests POST requests with body
func TestGlobalFetchPOST(t *testing.T) {
	// Create a test server that echoes the request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Read body
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)

		// Echo back request info
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Escape the body as a JSON string
		bodyBytes, _ := json.Marshal(string(body))
		fmt.Fprintf(w, `{
			"method": "%s",
			"contentType": "%s",
			"body": %s
		}`, r.Method, r.Header.Get("Content-Type"), string(bodyBytes))
	}))
	defer server.Close()

	p := NewPaserati()

	tsCode := fmt.Sprintf(`
		console.log("Testing POST request...");

		async function runTest() {
			const data = { name: "test", value: 42 };

			const response = await fetch("%s/api/test", {
				method: "POST",
				headers: {
					"Content-Type": "application/json"
				},
				body: JSON.stringify(data)
			});

			console.log("Response status:", response.status);

			const result = await response.json();
			console.log("Echo result:", result);

			// Verify the server received our data
			const echoedBody = JSON.parse(result.body);
			return echoedBody.name === "test" && echoedBody.value === 42 ? "post_test_passed" : "post_test_failed";
		}

		await runTest();
	`, server.URL)

	result, errs := p.RunCode(tsCode, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("Failed to run POST test: %v", errs[0])
	}

	if result.ToString() != "post_test_passed" {
		t.Errorf("Expected 'post_test_passed', got: %v", result.ToString())
	}
}

// TestGlobalFetchAutoStringify tests automatic JSON stringification of objects
func TestGlobalFetchAutoStringify(t *testing.T) {
	// Create a test server that echoes the request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Read body
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)

		// Echo back request info
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Escape the body as a JSON string
		bodyBytes, _ := json.Marshal(string(body))
		fmt.Fprintf(w, `{
			"method": "%s",
			"contentType": "%s",
			"body": %s
		}`, r.Method, r.Header.Get("Content-Type"), string(bodyBytes))
	}))
	defer server.Close()

	p := NewPaserati()

	tsCode := fmt.Sprintf(`
		console.log("Testing automatic JSON stringification...");

		async function runTest() {
			const data = { name: "test", value: 42 };

			// Pass object directly without JSON.stringify() - should auto-stringify
			const response = await fetch("%s/api/test", {
				method: "POST",
				headers: {
					"Content-Type": "application/json"
				},
				body: data  // Object passed directly
			});

			console.log("Response status:", response.status);

			const result = await response.json();
			console.log("Echo result:", result);

			// Verify the server received our data as JSON
			const echoedBody = JSON.parse(result.body);
			return echoedBody.name === "test" && echoedBody.value === 42 ? "auto_stringify_test_passed" : "auto_stringify_test_failed";
		}

		await runTest();
	`, server.URL)

	result, errs := p.RunCode(tsCode, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("Failed to run auto-stringify test: %v", errs[0])
	}

	if result.ToString() != "auto_stringify_test_passed" {
		t.Errorf("Expected 'auto_stringify_test_passed', got: %v", result.ToString())
	}
}

// TestGlobalFetchBlob tests the Response.blob() method which returns Uint8Array
func TestGlobalFetchBlob(t *testing.T) {
	// Create a test server that returns binary data
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Return some binary data
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{1, 2, 3, 4, 5, 255}) // Binary data including high byte value
	}))
	defer server.Close()

	p := NewPaserati()

	tsCode := fmt.Sprintf(`
		console.log("Testing blob() method...");

		async function runTest() {
			const response = await fetch("%s/binary");
			console.log("Response status:", response.status);

			// Get response as binary data (should be Uint8Array)
			const blob = await response.blob();
			console.log("Blob type:", typeof blob);
			console.log("Blob constructor:", blob.constructor?.name);
			console.log("Blob length:", blob.length);
			console.log("Blob byteLength:", blob.byteLength);
			console.log("Blob buffer:", typeof blob.buffer);

			// Check if it's a proper Uint8Array
			const hasUint8ArrayFeatures = blob.length === 6 &&
										  blob[0] === 1 &&
										  blob[5] === 255 &&
										  blob.byteLength === 6 &&
										  typeof blob.buffer === "object";

			return hasUint8ArrayFeatures ? "blob_test_passed" : "blob_test_failed";
		}

		await runTest();
	`, server.URL)

	result, errs := p.RunCode(tsCode, RunOptions{})
	if len(errs) > 0 {
		t.Fatalf("Failed to run blob test: %v", errs[0])
	}

	if result.ToString() != "blob_test_passed" {
		t.Errorf("Expected 'blob_test_passed', got: %v", result.ToString())
	}
}
