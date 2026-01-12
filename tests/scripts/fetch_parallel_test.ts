// Test that fetch runs truly in parallel
// Three 1-second delays should complete in ~1-2s if parallel, ~3s+ if sequential

async function test() {
    const start = Date.now();

    // These should run in parallel
    const [r1, r2, r3] = await Promise.all([
        fetch("https://httpbin.org/delay/1"),
        fetch("https://httpbin.org/delay/1"),
        fetch("https://httpbin.org/delay/1")
    ]);

    const elapsed = Date.now() - start;
    console.log("Elapsed:", elapsed, "ms");
    console.log("Statuses:", r1.status, r2.status, r3.status);

    // If parallel, should take ~1-2s. If sequential, ~3s+.
    // Use 2500ms as threshold to allow for network variance
    if (elapsed < 2500 && r1.status === 200 && r2.status === 200 && r3.status === 200) {
        return "parallel_fetch_passed";
    } else {
        return "parallel_fetch_failed";
    }
}

await test();

// expect: parallel_fetch_passed
