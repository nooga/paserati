// expect: simple instance test

// Test simple case without InstanceType to verify class works

class SimpleClass {
    name: string;
    constructor(name: string) {
        this.name = name;
    }
}

function test() {
    // Test direct class usage
    let instance: SimpleClass = new SimpleClass("Alice"); // This should work
    let name = instance.name; // This should work
    
    return "success";
}

test();

"simple instance test";