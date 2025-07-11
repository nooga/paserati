// expect: typeof basic test

// Test basic typeof type operator

let myVar = "hello";

// Use typeof in function context where myVar is already processed
function test() {
    // This should extract the type of myVar (which is string)
    let test: typeof myVar = "world";  // Should be valid since typeof myVar is string
    return test;
}

"typeof basic test";