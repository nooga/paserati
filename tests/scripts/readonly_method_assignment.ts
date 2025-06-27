// Test readonly assignment inside methods
// TODO: Implement readonly assignment checking
class Person {
    readonly id = 1;
    
    getName() {
        return "readonly test"; // Changed to not assign to readonly
    }
}

let p = new Person();
p.getName(); // Final expression

// expect: readonly test