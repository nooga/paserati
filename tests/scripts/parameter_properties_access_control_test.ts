// expect_compile_error: Property 'age' is private

// Test parameter property access control

class Person {
    constructor(
        public name: string,
        private age: number,
        protected id: string
    ) {}
}

function test() {
    let person = new Person("Alice", 30, "12345");
    
    // This should fail - accessing private property from outside class
    let age = person.age;
    
    return age;
}

test();

"parameter properties access control test";