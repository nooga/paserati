// expect: 5 1
// Regression test for paserati#292: a computed class field whose initializer
// is an arrow function with a block body, with no explicit trailing
// semicolon, was not correctly terminated by ASI when the very next class
// member also had a computed key - e.g.
//   [k1] = (x) => { return x }
//   [k2] () { ... }
// The generic Pratt expression parser doesn't know that an ArrowFunction is
// not a MemberExpression/LeftHandSideExpression, so it kept treating the
// next line's `[k2]` as a postfix index/member-access continuation of the
// arrow function (real engines reject `(x) => {}[0]` outright, with or
// without a line break in between) and swallowed the whole next member.
// Real, unmodified npm `undici` (a direct dependency of many packages) hits
// this exact shape in lib/dispatcher/pool-base.js.
const k1 = Symbol("e");
const k2 = Symbol("busy");

class A {
  [k1] = (x) => {
    return x;
  }

  [k2]() {
    return 1;
  }
}

const a = new A();
`${a[k1](5)} ${a[k2]()}`;
