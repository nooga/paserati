// expect: 1,*,5,method-set,method-get,20,1,*
// Regression test for #101: class fields named get/set with a same-line
// terminator (';', not a newline) must parse as fields, not accessors.

class Foo {
  set;
  pattern;
  constructor() {
    this.set = [1];
    this.pattern = "*";
  }
}
const foo = new Foo();

class Bar {
  get;
  x = 5;
}
const bar = new Bar();

class Baz {
  set() {
    return "method-set";
  }
}

class Qux {
  get() {
    return "method-get";
  }
}

// Real accessors must still work.
class Real {
  #v = 10;
  get x() {
    return this.#v;
  }
  set x(v: number) {
    this.#v = v;
  }
}
const real = new Real();
real.x = 20;

// Newline ASI form (already worked) must keep working.
class Asi {
  set
  pattern
  constructor() {
    this.set = [1];
    this.pattern = "*";
  }
}
const asi = new Asi();

[
  foo.set[0],
  foo.pattern,
  bar.x,
  new Baz().set(),
  new Qux().get(),
  real.x,
  asi.set[0],
  asi.pattern,
].join(",");
