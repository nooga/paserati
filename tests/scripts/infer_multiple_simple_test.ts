// expect: infer multiple simple test

// Test multiple infer sites without using the inferred type in true branch

// Simple test that should work
type ExtractFirstParam<T> = T extends (a: infer U, b: infer U) => any ? string : never;

// Test with function that has different parameter types
type TestFunc = (a: string, b: number) => void;

function test() {
    // This should be string (we're not using U, just testing the inference works)
    type Result = ExtractFirstParam<TestFunc>;
    
    let value: Result = "test"; // Should work since Result is string
    return value;
}

test();

"infer multiple simple test";