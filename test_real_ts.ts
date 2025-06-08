function Person(this: { name: string }, name: string) {
    this.name = name;
}

Person.prototype.greet = function(this: { name: string }) {
    return "Hello, " + this.name;
};

let alice = new Person("Alice");
alice.greet();