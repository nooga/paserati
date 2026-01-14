// FIXME: super property access in eval not yet supported
// expect_runtime_error: super expression can only be used within a class or object method
// Test: super property access in eval within field initializer

class A {
  x = 42;
}

var C = class extends A {
  field = eval('super.x');
};

const instance = new C();
instance.field === 42 ? "done" : "fail: " + instance.field
