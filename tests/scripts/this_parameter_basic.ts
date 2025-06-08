// Test explicit 'this' parameter support for constructor functions

// expect: {name: "Alice", age: 30}

function Person(this: { name: string; age: number }, name: string, age: number) {
    this.name = name;
    this.age = age;
}

let alice = new Person("Alice", 30);
alice