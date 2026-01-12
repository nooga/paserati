// Test complete fetch API: Blob, FormData, Request, Response constructors, iterators

async function runTests() {
    let passed = 0;
    let failed = 0;

    // Test 1: Blob constructor
    console.log("1. Testing Blob constructor...");
    try {
        const blob = new Blob(["Hello, ", "World!"], { type: "text/plain" });
        console.log("Blob size:", blob.size);
        console.log("Blob type:", blob.type);
        const text = await blob.text();
        console.log("Blob text:", text);

        if (blob.size === 13 && blob.type === "text/plain" && text === "Hello, World!") {
            console.log("Blob test passed");
            passed++;
        } else {
            console.log("Blob test failed");
            failed++;
        }
    } catch (error) {
        console.log("Blob test error:", error);
        failed++;
    }

    // Test 2: Blob.slice()
    console.log("\n2. Testing Blob.slice()...");
    try {
        const blob = new Blob(["0123456789"]);
        const sliced = blob.slice(2, 5);
        const text = await sliced.text();
        console.log("Sliced text:", text);

        if (sliced.size === 3 && text === "234") {
            console.log("Blob.slice test passed");
            passed++;
        } else {
            console.log("Blob.slice test failed");
            failed++;
        }
    } catch (error) {
        console.log("Blob.slice test error:", error);
        failed++;
    }

    // Test 3: FormData
    console.log("\n3. Testing FormData...");
    try {
        const formData = new FormData();
        formData.append("name", "John");
        formData.append("age", "30");
        formData.append("name", "Jane"); // Multiple values for same key

        console.log("FormData has name:", formData.has("name"));
        console.log("FormData get name:", formData.get("name"));
        console.log("FormData getAll name:", formData.getAll("name"));

        const hasName = formData.has("name");
        const getName = formData.get("name") === "John";
        const getAllName = formData.getAll("name").length === 2;

        if (hasName && getName && getAllName) {
            console.log("FormData test passed");
            passed++;
        } else {
            console.log("FormData test failed");
            failed++;
        }
    } catch (error) {
        console.log("FormData test error:", error);
        failed++;
    }

    // Test 4: Request constructor
    console.log("\n4. Testing Request constructor...");
    try {
        const request = new Request("https://example.com/api", {
            method: "POST"
        });

        console.log("Request method:", request.method);
        console.log("Request url:", request.url);

        if (request.method === "POST" && request.url === "https://example.com/api") {
            console.log("Request constructor test passed");
            passed++;
        } else {
            console.log("Request constructor test failed");
            failed++;
        }
    } catch (error) {
        console.log("Request constructor test error:", error);
        failed++;
    }

    // Test 5: Request.clone()
    console.log("\n5. Testing Request.clone()...");
    try {
        const request = new Request("https://example.com");
        const cloned = request.clone();

        console.log("Original url:", request.url);
        console.log("Cloned url:", cloned.url);

        if (cloned.url === request.url) {
            console.log("Request.clone test passed");
            passed++;
        } else {
            console.log("Request.clone test failed");
            failed++;
        }
    } catch (error) {
        console.log("Request.clone test error:", error);
        failed++;
    }

    // Test 6: Response constructor
    console.log("\n6. Testing Response constructor...");
    try {
        const response = new Response("Hello Response", {
            status: 201,
            statusText: "Created",
            headers: { "X-Custom": "value" }
        });

        console.log("Response status:", response.status);
        console.log("Response statusText:", response.statusText);
        console.log("Response ok:", response.ok);

        const text = await response.text();
        console.log("Response body:", text);

        if (response.status === 201 && response.ok && text === "Hello Response") {
            console.log("Response constructor test passed");
            passed++;
        } else {
            console.log("Response constructor test failed");
            failed++;
        }
    } catch (error) {
        console.log("Response constructor test error:", error);
        failed++;
    }

    // Test 7: Response.json() static method
    console.log("\n7. Testing Response.json()...");
    try {
        const response = Response.json({ message: "Hello JSON" }, { status: 200 });
        console.log("Response status:", response.status);

        const data = await response.json();
        console.log("JSON message:", data.message);

        if (response.status === 200 && data.message === "Hello JSON") {
            console.log("Response.json test passed");
            passed++;
        } else {
            console.log("Response.json test failed");
            failed++;
        }
    } catch (error) {
        console.log("Response.json test error:", error);
        failed++;
    }

    // Test 8: Response.redirect()
    console.log("\n8. Testing Response.redirect()...");
    try {
        const response = Response.redirect("https://example.com/new", 301);
        console.log("Redirect status:", response.status);
        console.log("Location header:", response.headers.get("Location"));

        if (response.status === 301 && response.headers.get("Location") === "https://example.com/new") {
            console.log("Response.redirect test passed");
            passed++;
        } else {
            console.log("Response.redirect test failed");
            failed++;
        }
    } catch (error) {
        console.log("Response.redirect test error:", error);
        failed++;
    }

    // Test 9: Response.error()
    console.log("\n9. Testing Response.error()...");
    try {
        const response = Response.error();
        console.log("Error response type:", response.type);
        console.log("Error response status:", response.status);

        if (response.type === "error" && response.status === 0) {
            console.log("Response.error test passed");
            passed++;
        } else {
            console.log("Response.error test failed");
            failed++;
        }
    } catch (error) {
        console.log("Response.error test error:", error);
        failed++;
    }

    // Test 10: Headers iterator with for...of
    console.log("\n10. Testing Headers iterator...");
    try {
        const headers = new Headers({
            "Content-Type": "application/json",
            "Accept": "text/html"
        });

        let count = 0;
        for (const [key, value] of headers) {
            console.log("Header:", key, "=", value);
            count++;
        }

        if (count >= 2) {
            console.log("Headers iterator test passed");
            passed++;
        } else {
            console.log("Headers iterator test failed - count:", count);
            failed++;
        }
    } catch (error) {
        console.log("Headers iterator test error:", error);
        failed++;
    }

    // Test 11: Headers.entries() returns iterator
    console.log("\n11. Testing Headers.entries() iterator...");
    try {
        const headers = new Headers({ "X-Test": "value" });
        const entries = headers.entries();

        // Check if it's an iterator (has next method)
        const first = entries.next();
        console.log("First entry done:", first.done);
        console.log("First entry value:", first.value);

        if (!first.done && Array.isArray(first.value)) {
            console.log("Headers.entries iterator test passed");
            passed++;
        } else {
            console.log("Headers.entries iterator test failed");
            failed++;
        }
    } catch (error) {
        console.log("Headers.entries iterator test error:", error);
        failed++;
    }

    console.log("\n=== Full Fetch API Tests Complete ===");
    console.log("Passed:", passed, "Failed:", failed);

    return passed >= 9 ? "full_fetch_api_tests_passed" : "full_fetch_api_tests_failed";
}

await runTests();

// expect: full_fetch_api_tests_passed
