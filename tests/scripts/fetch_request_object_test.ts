// skip: network
// fetch(request) must pull url/method/headers/body/signal from a Request
// instance instead of stringifying it into the URL (which used to produce
// "[object Object]" and silently drop every option on the Request).

async function runTests() {
    let passed = 0;
    let failed = 0;

    // Test 1: fetch(request) with no init - options come entirely from the Request.
    console.log("1. Testing fetch(request) with no init...");
    try {
        const req = new Request("https://httpbin.org/anything/one", {
            method: "POST",
            headers: { "X-Test": "hello" },
            body: "abc123",
        });
        const response = await fetch(req);
        const json = await response.json();
        console.log("method:", json.method);
        console.log("body:", json.data);
        console.log("header:", json.headers["X-Test"]);

        if (json.method === "POST" && json.data === "abc123" && json.headers["X-Test"] === "hello") {
            console.log("fetch(request) test passed");
            passed++;
        } else {
            console.log("fetch(request) test failed");
            failed++;
        }
    } catch (error) {
        console.log("fetch(request) test error:", error);
        failed++;
    }

    // Test 2: fetch(request, init) - explicit init overrides the Request's own fields.
    console.log("\n2. Testing fetch(request, init) override...");
    try {
        const req = new Request("https://httpbin.org/anything/two", {
            method: "GET",
            headers: { "X-Test": "original" },
        });
        const response = await fetch(req, {
            method: "PUT",
            headers: { "X-Test": "overridden" },
            body: "newbody",
        });
        const json = await response.json();
        console.log("method:", json.method);
        console.log("body:", json.data);
        console.log("header:", json.headers["X-Test"]);

        if (json.method === "PUT" && json.data === "newbody" && json.headers["X-Test"] === "overridden") {
            console.log("fetch(request, init) override test passed");
            passed++;
        } else {
            console.log("fetch(request, init) override test failed");
            failed++;
        }
    } catch (error) {
        console.log("fetch(request, init) override test error:", error);
        failed++;
    }

    console.log("\n=== Fetch Request Object Tests Complete ===");
    console.log("Passed:", passed, "Failed:", failed);

    return passed === 2 ? "fetch_request_object_tests_passed" : "fetch_request_object_tests_failed";
}

await runTests();

// expect: fetch_request_object_tests_passed
