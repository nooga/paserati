// expect_compile_error: unknown type name

// Test multiple infer sites in function parameters
// Note: This currently doesn't work because infer type parameters aren't 
// properly scoped in the true branch of conditional types

// Test 1: Multiple infer U in function parameters
type ExtractParams<T> = T extends (a: infer U, b: infer U) => any ? U : never;

// Test with function that has different parameter types
type TestFunc = (a: string, b: number) => void;

function test() {
    // This should be string | number but currently fails with "unknown type name: U"
    type Result = ExtractParams<TestFunc>;
    
    let value: Result = "test";
    return value;
}

test();

"infer array multiple test";