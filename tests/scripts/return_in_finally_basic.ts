// Test 1: Finally return overrides try return
function test1() {
    try {
        return "try";
    } finally {
        return "finally";
    }
}

test1();
// expect: finally