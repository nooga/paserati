// Regression test for issue #481: super.method(...args) must preserve 'this'
// binding to the actual instance (so instance fields are visible), not lose
// it to whatever compiling the SuperExpression object produced.
// expect: 42
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
  call() {
    const args = [10, 2];
    return super.method(...args);
  }
}

new Sub(30).call();
