// expect: undefined function
// Regression test for paserati#292 (same root cause, different trigger): a
// computed class field with no initializer but a type annotation whose own
// last token is '}' - an object type literal - and no trailing semicolon,
// hit the exact same bug as the arrow-function-initializer case in
// class_computed_field_arrow_asi.ts. parseComputedProperty's "no
// initializer" branch used to skip advancing past the current token
// whenever it was already a '}', assuming that could only be the class
// body's own closing brace; here it's the type annotation's closing brace
// instead, and stopping there made parseClassBody mistake it for the end of
// the class, swallowing the next computed member entirely.
const k1 = Symbol("a");
const k2 = Symbol("b");

class A {
  [k1]: { a: number }

  [k2]() {
    return 1;
  }
}

const a = new A();
`${typeof a[k1]} ${typeof a[k2]}`;
