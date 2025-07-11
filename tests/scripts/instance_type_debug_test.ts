// expect: instance type debug test

// Debug InstanceType<T> utility type

class SimpleClass {
    name: string;
    constructor(name: string) {
        this.name = name;
    }
}

function test() {
    // Test InstanceType extraction
    type SimpleInstance = InstanceType<typeof SimpleClass>; // Should be SimpleClass
    
    // Test assignment - this should reveal the issue
    let instance: SimpleInstance = new SimpleClass("Alice"); // This should work
    
    return "success";
}

test();

"instance type debug test";