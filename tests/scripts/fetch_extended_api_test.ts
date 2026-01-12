// Test extended fetch API features: AbortController, Headers iterators, Response clone, etc.

async function runTests() {
    let passed = 0;
    let failed = 0;

    // Test 1: AbortController creation
    console.log("1. Testing AbortController creation...");
    try {
        const controller = new AbortController();
        console.log("Controller created:", typeof controller);
        console.log("Signal:", typeof controller.signal);
        console.log("Signal.aborted:", controller.signal.aborted);

        if (controller.signal && controller.signal.aborted === false) {
            console.log("AbortController test passed");
            passed++;
        } else {
            console.log("AbortController test failed");
            failed++;
        }
    } catch (error) {
        console.log("AbortController test error:", error);
        failed++;
    }

    // Test 2: AbortController.abort()
    console.log("\n2. Testing AbortController.abort()...");
    try {
        const controller = new AbortController();
        console.log("Before abort - aborted:", controller.signal.aborted);

        controller.abort("Custom reason");
        console.log("After abort - aborted:", controller.signal.aborted);
        console.log("Reason:", controller.signal.reason);

        if (controller.signal.aborted === true) {
            console.log("Abort test passed");
            passed++;
        } else {
            console.log("Abort test failed");
            failed++;
        }
    } catch (error) {
        console.log("Abort test error:", error);
        failed++;
    }

    // Test 3: AbortSignal.abort() static method
    console.log("\n3. Testing AbortSignal.abort()...");
    try {
        const signal = AbortSignal.abort("Already aborted");
        console.log("Signal.aborted:", signal.aborted);

        if (signal.aborted === true) {
            console.log("AbortSignal.abort test passed");
            passed++;
        } else {
            console.log("AbortSignal.abort test failed");
            failed++;
        }
    } catch (error) {
        console.log("AbortSignal.abort test error:", error);
        failed++;
    }

    // Test 4: Headers.entries()
    console.log("\n4. Testing Headers.entries()...");
    try {
        const headers = new Headers({
            "Content-Type": "application/json",
            "X-Custom": "value"
        });

        const entries = headers.entries();
        console.log("Entries:", entries);
        console.log("Entries length:", entries.length);

        if (entries.length >= 2) {
            console.log("Headers.entries test passed");
            passed++;
        } else {
            console.log("Headers.entries test failed");
            failed++;
        }
    } catch (error) {
        console.log("Headers.entries test error:", error);
        failed++;
    }

    // Test 5: Headers.keys() and values()
    console.log("\n5. Testing Headers.keys() and values()...");
    try {
        const headers = new Headers({
            "Accept": "application/json",
            "Authorization": "Bearer token"
        });

        const keys = headers.keys();
        const values = headers.values();
        console.log("Keys:", keys);
        console.log("Values:", values);

        if (keys.length >= 2 && values.length >= 2) {
            console.log("Headers.keys/values test passed");
            passed++;
        } else {
            console.log("Headers.keys/values test failed");
            failed++;
        }
    } catch (error) {
        console.log("Headers.keys/values test error:", error);
        failed++;
    }

    // Test 6: Headers.forEach()
    console.log("\n6. Testing Headers.forEach()...");
    try {
        const headers = new Headers({
            "X-Test": "test-value"
        });

        let forEachCalled = false;
        headers.forEach((value, key) => {
            console.log("forEach:", key, "=", value);
            forEachCalled = true;
        });

        if (forEachCalled) {
            console.log("Headers.forEach test passed");
            passed++;
        } else {
            console.log("Headers.forEach test failed");
            failed++;
        }
    } catch (error) {
        console.log("Headers.forEach test error:", error);
        failed++;
    }

    // Test 7: Response properties (redirected, type)
    console.log("\n7. Testing Response properties...");
    try {
        const response = await fetch("https://httpbin.org/get");
        console.log("Response.redirected:", response.redirected);
        console.log("Response.type:", response.type);

        if (typeof response.redirected === "boolean" && typeof response.type === "string") {
            console.log("Response properties test passed");
            passed++;
        } else {
            console.log("Response properties test failed");
            failed++;
        }
    } catch (error) {
        console.log("Response properties test error:", error);
        failed++;
    }

    // Test 8: Response.clone()
    console.log("\n8. Testing Response.clone()...");
    try {
        const response = await fetch("https://httpbin.org/get");
        const clone = response.clone();

        console.log("Original bodyUsed:", response.bodyUsed);
        console.log("Clone bodyUsed:", clone.bodyUsed);
        console.log("Clone status:", clone.status);

        // Read from clone
        const cloneText = await clone.text();
        console.log("Clone text length:", cloneText.length);

        // Original should still be usable
        const originalText = await response.text();
        console.log("Original text length:", originalText.length);

        if (cloneText.length > 0 && originalText.length > 0) {
            console.log("Response.clone test passed");
            passed++;
        } else {
            console.log("Response.clone test failed");
            failed++;
        }
    } catch (error) {
        console.log("Response.clone test error:", error);
        failed++;
    }

    // Test 9: Response.arrayBuffer()
    console.log("\n9. Testing Response.arrayBuffer()...");
    try {
        const response = await fetch("https://httpbin.org/bytes/5");
        const buffer = await response.arrayBuffer();
        console.log("ArrayBuffer type:", typeof buffer);
        console.log("ArrayBuffer byteLength:", buffer.byteLength);

        if (buffer.byteLength === 5) {
            console.log("Response.arrayBuffer test passed");
            passed++;
        } else {
            console.log("Response.arrayBuffer test failed");
            failed++;
        }
    } catch (error) {
        console.log("Response.arrayBuffer test error:", error);
        failed++;
    }

    // Test 10: Response.bytes()
    console.log("\n10. Testing Response.bytes()...");
    try {
        const response = await fetch("https://httpbin.org/bytes/3");
        const bytes = await response.bytes();
        console.log("Bytes type:", typeof bytes);
        console.log("Bytes length:", bytes.length);

        if (bytes.length === 3) {
            console.log("Response.bytes test passed");
            passed++;
        } else {
            console.log("Response.bytes test failed");
            failed++;
        }
    } catch (error) {
        console.log("Response.bytes test error:", error);
        failed++;
    }

    // Test 11: Fetch with pre-aborted signal
    console.log("\n11. Testing fetch with pre-aborted signal...");
    try {
        const controller = new AbortController();
        controller.abort();

        await fetch("https://httpbin.org/get", { signal: controller.signal });
        console.log("Pre-aborted signal test failed - should have thrown");
        failed++;
    } catch (error) {
        console.log("Correctly caught abort error:", error);
        console.log("Pre-aborted signal test passed");
        passed++;
    }

    // Test 12: Redirect tracking
    console.log("\n12. Testing redirect tracking...");
    try {
        // httpbin.org/redirect/1 redirects once
        const response = await fetch("https://httpbin.org/redirect/1");
        console.log("Status:", response.status);
        console.log("Redirected:", response.redirected);
        console.log("Final URL:", response.url);

        if (response.redirected === true && response.status === 200) {
            console.log("Redirect tracking test passed");
            passed++;
        } else {
            console.log("Redirect tracking test failed");
            failed++;
        }
    } catch (error) {
        console.log("Redirect tracking test error:", error);
        failed++;
    }

    console.log("\n=== Extended Fetch API Tests Complete ===");
    console.log("Passed:", passed, "Failed:", failed);

    return passed >= 10 ? "extended_fetch_tests_passed" : "extended_fetch_tests_failed";
}

await runTests();

// expect: extended_fetch_tests_passed
