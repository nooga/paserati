// Test 3: Finally without return preserves try return
function test3() {
    try {
        return "try";
    } finally {
        // No return here - should preserve try return
    }
}

test3();
// expect: try