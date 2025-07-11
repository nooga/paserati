// expect: parameter properties parsing test

// Test parameter property parsing and type checking

class TestClass {
    constructor(public name: string, private age: number, protected id: string, readonly active: boolean) {
        // Constructor body
    }
}

function test() {
    let instance = new TestClass("Alice", 30, "12345", true);
    
    // Test access to synthesized properties
    let name = instance.name;        // Should work (public)
    // let age = instance.age;       // Should fail (private) - will be tested separately
    // let id = instance.id;         // Should fail (protected) - will be tested separately  
    let active = instance.active;    // Should work (readonly is accessible)
    
    return name;
}

test();

"parameter properties parsing test";