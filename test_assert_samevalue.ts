// Simulate the test262 assert.sameValue call
function sameValue(a: any, b: any) {
    console.log("Comparing:", a, "with:", b);
    return a === b;
}

let assert = { sameValue };

// This is similar to what's in the failing test262 test
function testcase() {
    assert.sameValue(1, 1); // Should work now with 2 arguments
}

testcase();
console.log("Test completed");