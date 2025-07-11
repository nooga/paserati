// expect: infer multiple working test

// Test that our multiple infer union logic works with builtin types

// Use a built-in conditional type that should handle multiple inferences
type TestMultiple<T> = T extends (a: infer U, b: infer U) => any ? "success" : "failed";

// Test with function that has different parameter types  
type TestFunc = (a: string, b: number) => void;

function test() {
    // This should be "success" since TestFunc matches the pattern
    type Result = TestMultiple<TestFunc>;
    
    let value: Result = "success"; // Should work
    return value;
}

test();

"infer multiple working test";