// paserati#453 - class X extends obj["key"] (a computed member access in a
// class's extends clause) used to be misparsed as a TypeScript type
// expression (indexed-access type / enum member access), instead of an
// ordinary runtime LeftHandSideExpression. This is the standard CJS/ESM
// interop idiom for unwrapping a default export (e.g. `lib["default"]`).
// expect: sub:base
// no-typecheck

const lib = { default: { Base: class Base { greet() { return "base"; } } } };

class Sub extends lib["default"].Base {
  greet() { return "sub:" + super.greet(); }
}

new Sub().greet();
