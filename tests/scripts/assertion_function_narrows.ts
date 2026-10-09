// expect: 6
function assertString(x: unknown): asserts x is string {
  if (typeof x !== "string") throw new Error("no");
}
function assertDefined<T>(x: T | undefined): asserts x {
  if (x === undefined) throw new Error("undefined");
}
function f(x: unknown): number { assertString(x); return x.length; }
function g(x: string | undefined): number { assertDefined(x); return x.length; }
f("abc") + g("def");
