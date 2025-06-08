// Check what currently works

function Person(this: { name: string }, name: string) {
    this.name = name;
}

// Check: Does Person.prototype return any?
let protoType = Person.prototype;
console.log("Person.prototype:", typeof protoType);

// Check: Can we assign to Person.prototype.greet?
Person.prototype.greet = function(this: { name: string }) {
    return "Hello, " + this.name;
};

// Check: Does new Person return any?
let alice = new Person("Alice");
console.log("alice type:", typeof alice);
console.log("alice value:", alice);

// Check: What happens when we access alice.greet?
console.log("alice.greet type:", typeof alice.greet);
alice.greet();