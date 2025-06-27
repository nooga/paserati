// Test current readonly capabilities
class Person {
    readonly name = "Alice";
    readonly age = 25;
}

let person = new Person();
console.log(person.name);
console.log(person.age);
person.name; // Final expression

// This should fail
// person.name = "Bob";

// expect: Alice