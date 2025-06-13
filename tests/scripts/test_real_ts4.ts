function Person(this: { name: string }, name: string) {
    this.name = name;
}

let alice = new Person("Alice");

// What type is alice?
let test1: {} = alice;           // Should work - alice is at least an object
let test2: { name: string } = alice;  // Does alice have the right shape?