// obj?.[expr](...) continuations must preserve `this` binding for method
// calls, just like the dot-access form obj?.prop(...) already does
// (issue #258, found while fixing #256).
//
// compileOptionalIndexExpression used to compile its continuation via
// compileOptionalContinuation, which always passes BadRegister as the
// receiver - so a method reached through a computed/bracket optional
// access lost `this` entirely, while the equivalent dot-access call
// worked correctly.

const o: any = {
  n: 7,
  m(a: number) {
    return this.n + a;
  },
  m2(a: number, b: number) {
    return this.n + a + b;
  },
};

// The core repro: obj?.["method"](args) must see `this` === obj.
const r1 = o?.["m"](5);

// Same object/method, reached via a computed (non-literal) key.
const key = "m";
const r2 = o?.[key](5);

// Nullish base must still short-circuit to undefined, not crash.
const n: any = null;
const r3 = n?.["m"](5);

// Multiple arguments still reach the method correctly alongside `this`.
const r4 = o?.["m2"](10, 100);

JSON.stringify([r1, r2, r3, r4]);
// expect: [12,12,null,117]
