// expect: sub:base
// #453 companion case: same member-expression superclass as
// bug_class_extends_member_expr_outer_scope.ts, but declared inside a
// function body where the outer `lib` binding's real type is already known
// by the time the class is checked. This exercises the `super.greet()`
// resolution fallback to the resolved superclass instance type (rather than
// the `any`-forward-reference path used at the top level).
function run() {
  const lib = { default: { Base: class Base { greet() { return "base"; } } } };

  class Sub extends lib.default.Base {
    greet() {
      return "sub:" + super.greet();
    }
  }

  return new Sub().greet();
}

run();
