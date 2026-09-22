// expect: Base ctor,x=42
// skip-typecheck
// paserati#504: a derived class with an instance field used to throw "Must
// call super constructor..." even though super() genuinely was called first
// - the only unusual thing was that it sat inside a comma expression that
// was itself the test of an `if` statement (a real shape minifiers produce
// by folding `super(); this.x = t;` into `if (super(), this.x = t, cond)`),
// rather than being its own top-level statement. Field initializers must
// run as part of evaluating the SuperCall itself, wherever it is, not only
// when it's textually a whole top-level statement or comma-chain statement.
let log: string[] = [];

class Base {
  constructor() {
    log.push("Base ctor");
  }
}

class Derived extends Base {
  x: number = 0;
  constructor(t: { x?: number } = {}) {
    if (super(), (this.x = t.x ?? 0), true) {
      log.push("x=" + this.x);
    }
  }
}

new Derived({ x: 42 });
log.join(",");
