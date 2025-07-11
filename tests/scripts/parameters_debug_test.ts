// expect: parameters debug test

// Debug Parameters<T> utility type

// Simple function to test
function simpleFunc(name: string, age: number): void {}

function test() {
    // Test basic Parameters extraction
    type SimpleParams = Parameters<typeof simpleFunc>; // Should be [string, number]
    
    // Test assignment
    let params: SimpleParams = ["Alice", 30]; // This should work
    
    return "success";
}

test();

"parameters debug test";