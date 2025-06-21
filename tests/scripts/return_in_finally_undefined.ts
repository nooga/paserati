// Test 7: Finally return undefined
function test7() {
    try {
        return "try";
    } finally {
        return; // Return undefined
    }
}

test7();
// expect: undefined