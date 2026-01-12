// Test advanced abort features: in-flight abort, AbortSignal.timeout(), AbortSignal.any()

async function runTests() {
    let passed = 0;
    let failed = 0;

    // Test 1: In-flight abort - abort during an active request
    // We use a slow endpoint and abort immediately after starting
    console.log("1. Testing in-flight abort...");
    try {
        const controller = new AbortController();

        // Start a slow request (2 second delay) and abort immediately
        // The abort should cancel the in-flight request
        const fetchPromise = fetch("https://httpbin.org/delay/2", {
            signal: controller.signal
        });

        // Abort immediately - the request is already in-flight
        controller.abort("Cancelled by user");

        // This should throw an AbortError
        await fetchPromise;
        console.log("In-flight abort test failed - should have thrown");
        failed++;
    } catch (error) {
        const errorStr = String(error);
        console.log("Caught error:", errorStr);
        if (errorStr.includes("Abort") || errorStr.includes("abort")) {
            console.log("In-flight abort test passed");
            passed++;
        } else {
            console.log("In-flight abort test failed - wrong error type:", errorStr);
            failed++;
        }
    }

    // Test 2: AbortSignal.timeout() - creates a signal that should abort after timeout
    console.log("\n2. Testing AbortSignal.timeout()...");
    try {
        const signal = AbortSignal.timeout(100); // 100ms timeout
        console.log("Timeout signal created");
        console.log("Initial aborted state:", signal.aborted);

        // The signal should not be aborted immediately
        if (signal.aborted === false) {
            console.log("AbortSignal.timeout creation test passed");
            passed++;
        } else {
            console.log("AbortSignal.timeout creation test failed - should not be immediately aborted");
            failed++;
        }
    } catch (error) {
        console.log("AbortSignal.timeout test error:", error);
        failed++;
    }

    // Test 3: AbortSignal.any() with non-aborted signals
    console.log("\n3. Testing AbortSignal.any() with non-aborted signals...");
    try {
        const controller1 = new AbortController();
        const controller2 = new AbortController();

        const combinedSignal = AbortSignal.any([controller1.signal, controller2.signal]);
        console.log("Combined signal aborted:", combinedSignal.aborted);

        if (combinedSignal.aborted === false) {
            console.log("AbortSignal.any (non-aborted) test passed");
            passed++;
        } else {
            console.log("AbortSignal.any (non-aborted) test failed");
            failed++;
        }
    } catch (error) {
        console.log("AbortSignal.any test error:", error);
        failed++;
    }

    // Test 4: AbortSignal.any() with one already-aborted signal
    console.log("\n4. Testing AbortSignal.any() with pre-aborted signal...");
    try {
        const controller1 = new AbortController();
        const controller2 = new AbortController();

        // Abort one of them first
        controller1.abort("First signal aborted");

        const combinedSignal = AbortSignal.any([controller1.signal, controller2.signal]);
        console.log("Combined signal aborted:", combinedSignal.aborted);

        if (combinedSignal.aborted === true) {
            console.log("AbortSignal.any (pre-aborted) test passed");
            passed++;
        } else {
            console.log("AbortSignal.any (pre-aborted) test failed - should be aborted");
            failed++;
        }
    } catch (error) {
        console.log("AbortSignal.any (pre-aborted) test error:", error);
        failed++;
    }

    // Test 5: AbortSignal.any() with AbortSignal.abort()
    console.log("\n5. Testing AbortSignal.any() with AbortSignal.abort()...");
    try {
        const alreadyAborted = AbortSignal.abort("Pre-aborted reason");
        const controller = new AbortController();

        const combinedSignal = AbortSignal.any([alreadyAborted, controller.signal]);
        console.log("Combined signal aborted:", combinedSignal.aborted);

        if (combinedSignal.aborted === true) {
            console.log("AbortSignal.any with abort() test passed");
            passed++;
        } else {
            console.log("AbortSignal.any with abort() test failed");
            failed++;
        }
    } catch (error) {
        console.log("AbortSignal.any with abort() test error:", error);
        failed++;
    }

    // Test 6: Using AbortSignal.any() with fetch
    console.log("\n6. Testing fetch with AbortSignal.any()...");
    try {
        const controller1 = new AbortController();
        const controller2 = new AbortController();

        // Abort controller1 immediately
        controller1.abort();

        const combinedSignal = AbortSignal.any([controller1.signal, controller2.signal]);

        await fetch("https://httpbin.org/get", { signal: combinedSignal });
        console.log("Fetch with AbortSignal.any test failed - should have thrown");
        failed++;
    } catch (error) {
        const errorStr = String(error);
        console.log("Caught error:", errorStr);
        if (errorStr.includes("Abort") || errorStr.includes("abort")) {
            console.log("Fetch with AbortSignal.any test passed");
            passed++;
        } else {
            console.log("Fetch with AbortSignal.any test failed - wrong error");
            failed++;
        }
    }

    // Test 7: Verify abort reason is preserved
    console.log("\n7. Testing abort reason preservation...");
    try {
        const controller = new AbortController();
        controller.abort("Custom abort reason");

        console.log("Signal aborted:", controller.signal.aborted);
        console.log("Signal reason:", controller.signal.reason);

        const reasonStr = String(controller.signal.reason);
        if (controller.signal.aborted && reasonStr.includes("Custom abort reason")) {
            console.log("Abort reason preservation test passed");
            passed++;
        } else {
            console.log("Abort reason preservation test failed");
            failed++;
        }
    } catch (error) {
        console.log("Abort reason test error:", error);
        failed++;
    }

    console.log("\n=== Advanced Abort Tests Complete ===");
    console.log("Passed:", passed, "Failed:", failed);

    return passed >= 6 ? "advanced_abort_tests_passed" : "advanced_abort_tests_failed";
}

await runTests();

// expect: advanced_abort_tests_passed
