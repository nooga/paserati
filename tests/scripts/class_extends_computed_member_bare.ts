// paserati#453 - the bare form (no trailing .prop after the computed
// access) hit a different downstream error before the fix:
// "compilation not implemented for *parser.IndexedAccessTypeExpression".
// expect: base
// no-typecheck

const lib = { default: class Base { greet() { return "base"; } } };

class Sub extends lib["default"] {}

new Sub().greet();
