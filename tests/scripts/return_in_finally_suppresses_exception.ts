// Test 2: Finally return suppresses exception
function test2() {
    try {
        throw new Error("error");
    } finally {
        return "finally";
    }
}

test2();
// expect: finally