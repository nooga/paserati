// expect: parameter properties comprehensive test

// Test comprehensive parameter property functionality

class Person {
    constructor(
        public name: string,
        private age: number,
        protected id: string,
        readonly active: boolean,
        public city?: string
    ) {
        // Constructor body (could have additional logic)
    }
    
    // Method to test access to private property from within class
    canAccessPrivate(): number {
        return this.age; // Should work - accessing private from within class
    }
    
    // Method to test access to protected property from within class
    canAccessProtected(): string {
        return this.id; // Should work - accessing protected from within class
    }
}

function test() {
    // Test parameter property assignment
    let person = new Person("Alice", 30, "12345", true, "NYC");
    
    // Test public property access
    let name = person.name;        // Should work: "Alice"
    let city = person.city;        // Should work: "NYC" (optional parameter)
    
    // Test readonly property access  
    let active = person.active;    // Should work: true
    
    // Test access to private/protected through methods
    let age = person.canAccessPrivate();      // Should work: 30
    let id = person.canAccessProtected();     // Should work: "12345"
    
    // Verify values are correctly assigned
    if (name !== "Alice") return "name assignment failed";
    if (city !== "NYC") return "city assignment failed";
    if (active !== true) return "active assignment failed";
    if (age !== 30) return "age assignment failed";
    if (id !== "12345") return "id assignment failed";
    
    return "all tests passed";
}

test();

"parameter properties comprehensive test";