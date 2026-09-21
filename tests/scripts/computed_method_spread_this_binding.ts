// Regression test: obj[key](...args) and super[key](...args) must preserve
// the 'this' binding used to look up the function, for both the fast
// single-spread path and the general multi-arg spread path.
// expect: true
// no-typecheck

class Base {
  method(a, b) {
    return this.value + a + b;
  }
}

class Sub extends Base {
  constructor(value) {
    super();
    this.value = value;
  }
  callSuperComputed() {
    const key = "method";
    return super[key](...[10, 2]);
  }
  callSuperComputedMulti() {
    const key = "method";
    return super[key](10, ...[2]);
  }
}

class Plain {
  constructor(value) {
    this.value = value;
  }
  method(a, b) {
    return this.value + a + b;
  }
}

const sub = new Sub(30);
const plain = new Plain(100);
const key = "method";

sub.callSuperComputed() === 42 &&
  sub.callSuperComputedMulti() === 42 &&
  plain[key](...[1, 2]) === 103 &&
  plain[key](1, ...[2]) === 103;
