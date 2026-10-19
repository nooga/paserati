// FIXME: super property access in eval not yet supported
// expect_runtime_error: 'super' can only be referenced in members of derived classes or object literal expressions
// Test: super property access in arrow function created by eval in field initializer

class A {
  x = 42;
}

var C = class extends A {
  getX = eval('() => super.x');
};

const instance = new C();
instance.getX()
