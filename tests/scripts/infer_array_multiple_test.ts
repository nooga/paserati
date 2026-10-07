// expect: infer array multiple test

// Test multiple infer sites in function parameters
// Note: infer type parameters are scoped to the true branch of the
// conditional type.

// Test 1: Multiple infer U in function parameters
type ExtractParams<T> = T extends (a: infer U, b: infer U) => any ? U : never;

// Test with function that has different parameter types
type TestFunc = (a: string, b: number) => void;

function test() {
    // This should be string | number and works now that infer is scoped
    type Result = ExtractParams<TestFunc>;
    
    let value: Result = "test";
    return value;
}

test();

"infer array multiple test";