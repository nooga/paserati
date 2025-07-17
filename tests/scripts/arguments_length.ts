// Test arguments object length property with different argument counts
// expect: all length tests passed

function testNoArgs() {
    return arguments.length;
}

function testOneArg(x: any) {
    return arguments.length;
}

function testThreeArgs(x: any, y: any, z: any) {
    return arguments.length;
}

function testManyArgs(a: any, b: any, c: any, d: any, e: any) {
    return arguments.length;
}

function runTests() {
    // Test no arguments
    if (testNoArgs() !== 0) {
        return `testNoArgs should return 0, got ${testNoArgs()}`;
    }
    
    // Test one argument
    if (testOneArg(5) !== 1) {
        return `testOneArg should return 1, got ${testOneArg(5)}`;
    }
    
    // Test three arguments
    if (testThreeArgs(1, 2, 3) !== 3) {
        return `testThreeArgs should return 3, got ${testThreeArgs(1, 2, 3)}`;
    }
    
    // Test many arguments
    if (testManyArgs("a", "b", "c", "d", "e") !== 5) {
        return `testManyArgs should return 5, got ${testManyArgs("a", "b", "c", "d", "e")}`;
    }
    
    return "all length tests passed";
}

runTests();