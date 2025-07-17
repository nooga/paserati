// Test typeof arguments object
// expect: typeof arguments test passed

function testArgumentsType(...args: any[]) {
    // arguments should be an object
    if (typeof arguments !== "object") {
        return `typeof arguments should be "object", got "${typeof arguments}"`;
    }
    
    // arguments should not be null
    if (arguments === null) {
        return "arguments should not be null";
    }
    
    return "typeof arguments test passed";
}

testArgumentsType("test", 123);