// Test arguments object indexed access
// expect: indexed access test passed

function testIndexedAccess(...args: any[]) {
    // Test accessing arguments at various indices
    if (arguments[0] !== "first") {
        return `arguments[0] should be "first", got ${arguments[0]}`;
    }
    
    if (arguments[1] !== 42) {
        return `arguments[1] should be 42, got ${arguments[1]}`;
    }
    
    if (arguments[2] !== true) {
        return `arguments[2] should be true, got ${arguments[2]}`;
    }
    
    if (arguments[3] !== null) {
        return `arguments[3] should be null, got ${arguments[3]}`;
    }
    
    // Test out of bounds access (should return undefined)
    if (typeof arguments[10] !== "undefined") {
        return `arguments[10] should be undefined, got ${typeof arguments[10]}`;
    }
    
    return "indexed access test passed";
}

testIndexedAccess("first", 42, true, null);