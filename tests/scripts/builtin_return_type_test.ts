// expect: builtin return type test

// Test built-in ReturnType utility

function testFunction(): string {
    return "hello";
}

function test() {
    // Use built-in ReturnType instead of defining our own
    type TestReturnType = ReturnType<typeof testFunction>; // Should be extracted from function
    
    // For now, this might just be 'any' but at least it should not error
    let result: TestReturnType = "test"; 
    return result;
}

test();

"builtin return type test";