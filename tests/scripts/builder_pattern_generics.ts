// Fluent API pattern with method chaining
// expect: John:30:NYC

// Person class with fluent setters (no explicit return type)
class Person {
  name: string = "";
  age: number = 0;
  city: string = "";

  setName(n: string) {
    this.name = n;
    return this;
  }

  setAge(a: number) {
    this.age = a;
    return this;
  }

  setCity(c: string) {
    this.city = c;
    return this;
  }
}

// Build a person using fluent API
const person = new Person()
  .setName("John")
  .setAge(30)
  .setCity("NYC");

// Access properties
person.name + ":" + person.age + ":" + person.city;
