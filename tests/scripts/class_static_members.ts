// expect: Counter: 3
// Test static fields and methods

class StaticExample {
  // Static fields
  static count = 0;
  static readonly version = "1.0";
  static instances = [];

  // Instance field
  name;

  constructor(name) {
    this.name = name;
    StaticExample.count++;
    StaticExample.instances.push(this);
  }

  // Static methods
  static getCount() {
    return StaticExample.count;
  }

  static reset() {
    StaticExample.count = 0;
    StaticExample.instances = [];
  }

  static createDefault() {
    return new StaticExample("default");
  }

  // Instance method
  toString() {
    return `${this.name} (#${StaticExample.count})`;
  }
}

let a = new StaticExample("A");
let b = new StaticExample("B");
let c = new StaticExample("C");

`Counter: ${StaticExample.getCount()}`;
