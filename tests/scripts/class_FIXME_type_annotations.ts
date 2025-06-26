// FIXME: Type annotations not yet supported
// expect: Alice
// Test basic property type annotations

class Person {
  name: string; // FIXME: Type annotation parsing
  age: number; // FIXME: Type annotation parsing

  constructor(name: string, age: number) {
    // FIXME: Parameter type annotations
    this.name = name;
    this.age = age;
  }

  getName(): string {
    // FIXME: Return type annotation
    return this.name;
  }
}

let person = new Person("Alice", 30);
person.getName();
