// expect_compile_error: Type 'string' does not satisfy the expected type 'number'.

// Test satisfies error handling - should catch type mismatches

function test() {
    // This should fail - string does not satisfy number constraint
    let value = "hello" satisfies number;
    
    return value;
}

test();

"satisfies error test";