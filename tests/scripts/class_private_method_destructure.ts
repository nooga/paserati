// no-typecheck
// expect: 1
// Test: Array destructuring in private method parameters

var callCount = 0;
var C = class {
  #method([x, y, z]) {
    if (x === 1 && y === 2 && z === 3) {
      callCount = callCount + 1;
    }
  }

  get method() {
    return this.#method;
  }
};

new C().method([1, 2, 3]);
callCount;
