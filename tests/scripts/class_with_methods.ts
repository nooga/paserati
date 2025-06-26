// expect: Hello, my name is Max and I'm a student
// Class with methods test
class Person {
  name;

  constructor(name) {
    this.name = name;
  }

  greet(message) {
    return `Hello, my name is ${this.name} ${message}`;
  }
}

let person = new Person("Max");
person.greet("and I'm a student");
