// Simplest possible derived class test
class Parent {
  constructor() {
    console.log("Parent constructor");
  }
}

class Child extends Parent {
  constructor() {
    console.log("Child constructor - before super");
    super();
    console.log("Child constructor - after super");
  }
}

console.log("Creating instance...");
const c = new Child();
console.log("Instance created:", c);
console.log("c [[Prototype]]:", Object.getPrototypeOf(c));
console.log("Child.prototype:", Child.prototype);
console.log("Match:", Object.getPrototypeOf(c) === Child.prototype);
