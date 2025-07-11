// expect: infer return type test

// Test ReturnType implementation
type ReturnType<T> = T extends (...args: any[]) => infer R ? R : never;

// Test with a known function type
type StringFunc = () => string;

function test() {
    // This should work - extract string from StringFunc
    type ExtractedType = ReturnType<StringFunc>; // Should be string
    let result: ExtractedType = "hello"; // Should work
    return result;
}

test();

"infer return type test";