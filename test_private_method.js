// no-typecheck
var C = class {
  #method([x, y, z]) {
    console.log("x:", x);
  }
  get method() {
    return this.#method;
  }
};
new C().method([1, 2, 3]);
