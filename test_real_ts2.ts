function Person(this: { name: string }, name: string) {
    this.name = name;
}

Person.prototype.greet = function(this: { name: string }) {
    return "Hello, " + this.name;
};

let alice = new Person("Alice");
// Let's see what type alice has
let test: { name: string } = alice; // Should work if alice has the right type
alice.greet(); // Should work if alice inherits from Person.prototype