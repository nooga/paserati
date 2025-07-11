// expect_compile_error: cannot assign to readonly property

// Test parameter property readonly enforcement

class Person {
    constructor(
        public name: string,
        readonly id: string
    ) {}
}

function test() {
    let person = new Person("Alice", "12345");
    
    // This should work - reading readonly property
    let id = person.id;
    
    // This should fail - attempting to modify readonly property
    person.id = "67890";
    
    return id;
}

test();

"parameter properties readonly test";