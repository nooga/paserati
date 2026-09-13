// expect: sub:base
// #453: a class's `extends` clause with a member-expression superclass (as
// opposed to a bare identifier) failed to resolve outer-scope identifiers.
// Top-level classes are type-checked (including their extends clause) in an
// earlier pass than the one that defines top-level const/let/var bindings,
// so `lib` wasn't yet defined in the environment when `lib.default.Base` was
// checked - this incorrectly raised TS2304 "Cannot find name 'lib'", and the
// subsequent `super.greet()` call failed with "could not resolve superclass".
const lib = { default: { Base: class Base { greet() { return "base"; } } } };

class Sub extends lib.default.Base {
  greet() {
    return "sub:" + super.greet();
  }
}

new Sub().greet();
