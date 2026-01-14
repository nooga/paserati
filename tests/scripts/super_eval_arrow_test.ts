// FIXME: super property access in eval not yet supported
// expect_runtime_error: super expression can only be used within a class or object method
// Test: super property access in arrow function created by eval in field initializer

class A {
  x = 42;
}

var C = class extends A {
  getX = eval('() => super.x');
};

const instance = new C();
instance.getX()
