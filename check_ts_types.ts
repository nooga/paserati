function Person(this: { name: string }, name: string) {
    this.name = name;
}

// What type is Person.prototype?
let proto: any = Person.prototype; // Should work if Person.prototype is any

Person.prototype.greet = function(this: { name: string }) {
    return "Hello, " + this.name;
};

let alice = new Person("Alice");

// What type is alice?
let aliceTest: any = alice; // Should work if alice is any

// Can we access greet?
alice.greet(); // Should work if alice is any