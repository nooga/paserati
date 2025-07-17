// Test basic arguments object functionality
// expect: basic arguments test passed

function testArguments(a: number, b: string) {
    // Check that arguments object exists
    if (typeof arguments === "undefined") {
        return "arguments object is undefined";
    }
    
    // Check arguments length
    if (arguments.length !== 2) {
        return `arguments.length should be 2, got ${arguments.length}`;
    }
    
    // Check indexed access
    if (arguments[0] !== a) {
        return `arguments[0] should be ${a}, got ${arguments[0]}`;
    }
    
    if (arguments[1] !== b) {
        return `arguments[1] should be ${b}, got ${arguments[1]}`;
    }
    
    return "basic arguments test passed";
}

testArguments(42, "hello");