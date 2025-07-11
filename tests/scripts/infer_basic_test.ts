// expect: infer basic test

// Test basic infer keyword functionality

// Simple ReturnType implementation using infer
type ReturnType<T> = T extends (...args: any[]) => infer R ? R : never;

// Test function
function testFunction(): string {
    return "hello";
}

function testInfer() {
    // Extract return type using infer
    type TestReturnType = ReturnType<typeof testFunction>; // Should be string
    
    // Use the inferred type
    let result: TestReturnType = "world"; // Should work since TestReturnType is string
    return result;
}

testInfer();

"infer basic test";