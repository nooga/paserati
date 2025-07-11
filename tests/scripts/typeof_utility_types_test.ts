// expect: typeof utility types test

// Test typeof with conditional types for function utility types

// Define a sample function at global scope
function sampleFunction(name: string, age: number): string {
    return name + " is " + age + " years old";
}

// Test basic typeof with conditional types (simplified without infer)
function testTypeof() {
    // Test that typeof works with the function type
    type FuncType = typeof sampleFunction; // Should be (name: string, age: number) => string
    
    // Test that we can use the typeof result
    let myFunc: FuncType = sampleFunction; // Should work
    return myFunc("test", 25);
}

testTypeof();

"typeof utility types test";