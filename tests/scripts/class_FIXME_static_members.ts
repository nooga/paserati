// FIXME: Static members not yet supported
// expect: Counter: 2
// Test static properties and methods

class Counter {
  static count = 0; // FIXME: static property
  static readonly max = 100; // FIXME: static readonly

  name;

  constructor(name) {
    this.name = name;
    Counter.increment(); // FIXME: static method call
  }

  static increment() {
    // FIXME: static method
    Counter.count++;
  }

  static getCount() {
    // FIXME: static method
    return Counter.count;
  }

  static reset() {
    // FIXME: static method
    Counter.count = 0;
  }
}

let a = new Counter("A");
let b = new Counter("B");
`Counter: ${Counter.getCount()}`;
