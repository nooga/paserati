// Test 4: Finally without return preserves exception
function test4() {
    try {
        throw new Error("original error");
    } catch (e) {
        throw new Error("rethrown error");
    } finally {
        // No return here - should preserve exception
    }
}

test4();
// expect_runtime_error: rethrown error