// expect: true
// Regression test for #389: an ES6 object-literal shorthand method (and a
// class method) must NOT bind its own name inside its body, unlike a
// genuine named function expression, which legitimately does.

function foo(x: number): string {
  return "outer foo called with " + x;
}

// Object-literal shorthand method: bare `foo` inside must resolve to the
// OUTER `foo`, not recurse into itself.
const obj = {
  foo(x: number): string {
    return foo(x);
  },
};
const objResult = obj.foo(42) === "outer foo called with 42";

// Class method: same rule applies.
class C {
  foo(x: number): string {
    return foo(x);
  }
}
const classResult = new C().foo(1) === "outer foo called with 1";

// Sanity check: a genuine named function expression DOES self-bind - the
// fix must not over-fire and break this legitimate case.
const rec = function fact(n: number): number {
  return n <= 1 ? 1 : n * fact(n - 1);
};
const namedFnExprResult = rec(5) === 120;

objResult && classResult && namedFnExprResult;
