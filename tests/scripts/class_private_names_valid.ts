// Valid private-name and field-initializer forms accepted alongside the class early errors
// skip-typecheck
class Outer {
  #a = 1;
  static #s = 10;
  get #p() { return 2; }
  set #p(v) {}
  static get #q() { return 3; }
  static set #q(v) {}
  f = function () { return arguments.length; };
  g = { m() { return arguments.length; } };
  sum() {
    const Inner = class {
      read(o) { return o.#a + o.#p + Outer.#q; }
    };
    return new Inner().read(this) + eval("this.#a + Outer.#s");
  }
  has(o) { return #a in o; }
}
const o = new Outer();
[o.sum(), o.f(1, 2), o.g.m(), o.has(o), o.has({})];

// expect: [17, 2, 0, true, false]
