// Test runtime inheritance (bypass type checker with any)
// expect: Hello, Alice

function Person(name: string) {
    (this as any).name = name;
}

(Person as any).prototype.greet = function() {
    return "Hello, " + (this as any).name;
};

let alice = new (Person as any)("Alice");
(alice as any).greet();