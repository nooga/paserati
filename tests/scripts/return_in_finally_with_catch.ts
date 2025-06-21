// Test 5: Finally return overrides catch return
function test5() {
    try {
        throw new Error("error");
    } catch (e) {
        return "catch";
    } finally {
        return "finally";
    }
}

test5();
// expect: finally