// expect_compile_error: unknown type name

// Test multiple infer sites with same name
// Note: Object type parsing now works, but using inferred type parameters 
// in the true branch still needs enhanced scoping support

// Test 1: Multiple infer U in object type should create union
type ExtractBoth<T> = T extends { a: infer U, b: infer U } ? U : never;

// Test with object that has different types for a and b
type TestObj = { a: string, b: number };

function test() {
    // This should be string | number (union of both inferred types)
    type Result = ExtractBoth<TestObj>;
    
    // Test that we can assign both string and number to Result
    let str: Result = "hello";  // Should work
    let num: Result = 42;       // Should work
    
    return str;
}

test();

"infer multiple sites test";