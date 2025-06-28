// expect_compile_error: cannot create an instance of an abstract class 'Shape'
// Test that abstract classes cannot be instantiated

abstract class Shape {
    name: string;
    
    constructor(name: string) {
        this.name = name;
    }
    
    // Abstract method - must be implemented by subclasses
    abstract area(): number;
}

// This should fail - trying to instantiate abstract class
let shape = new Shape("test");