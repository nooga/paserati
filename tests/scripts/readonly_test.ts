// Test readonly modifier in classes
class Person {
  readonly id;
  name;
  readonly age = 25;

  constructor(id, name) {
    this.id = id;
    this.name = name;
  }

  updateName(newName) {
    this.name = newName; // Should work
  }
}

let person = new Person(1, "Alice");
console.log(person.id); // Should work - reading readonly
console.log(person.name); // Should work - reading non-readonly
console.log(person.age); // Should work - reading readonly with initializer

person.age;
// expect: 25
