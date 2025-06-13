// Test simple prototype inheritance without explicit this types
// expect: Hello, Alice

function Person(name: string) {
    (this as any).name = name;
}

(Person as any).prototype.greet = function() {
    return "Hello, " + (this as any).name;
};

let alice = new (Person as any)("Alice");
(alice as any).greet();