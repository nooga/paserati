// A second (or later) call chained onto an optional-chain continuation's
// first call must still see the right `this` - not just the very first
// call in the chain (issue #260, found while chasing #256/#258).
//
// compileOptionalContinuationWithReceiver used to gate method-call `this`
// binding on `isFirstCall` (node.Function == nil), so only the innermost
// call in the continuation ever got a receiver; every later call chained
// onto a property access lost `this` and became a plain call.

const o: any = {
  n: 1,
  add(x: number) {
    this.n += x;
    return this;
  },
};

// The core repro: obj?.method(a).method(b) - the second .add() must still
// see `this` === o.
o?.add(1).add(2);
const r1 = o.n; // 4

// Three deep, to make sure the fix isn't a one-level special case.
const o2: any = {
  n: 10,
  add(x: number) {
    this.n += x;
    return this;
  },
};
o2?.add(1).add(2).add(3);
const r2 = o2.n; // 16

// Bracket-access second call: obj?.method(a)["method"](b).
const o3: any = { n: 100, add(x: number) { this.n += x; return this; } };
o3?.add(1)["add"](2);
const r3 = o3.n; // 103

// Bracket-access FIRST call (the #258 shape) with a dot-access second call,
// chained together - confirms this is a distinct fix site from #258's.
const o4: any = { n: 1000, add(x: number) { this.n += x; return this; } };
o4?.["add"](1).add(2);
const r4 = o4.n; // 1003

// Spread arguments on the second call.
const o5: any = {
  n: 0,
  add(...xs: number[]) {
    this.n += xs.reduce((a, b) => a + b, 0);
    return this;
  },
};
o5?.add(1).add(...[2, 3, 4]);
const r5 = o5.n; // 10

// obj?.b()() - the callee of the second call is a call's *return value*,
// not a property access, so it correctly gets NO `this` binding (matches
// plain JS semantics for calling a returned function directly).
function makeAdder(base: number) {
  return function (x: number) {
    return base + x;
  };
}
const holder: any = { make: makeAdder };
const r6 = holder?.make(10)(5); // 15

// Nullish base still short-circuits the whole chain to undefined, even
// with multiple chained calls following it.
const n: any = null;
const r7 = n?.add(1).add(2); // undefined

// a?.b.method() - a plain (uncalled) member access immediately followed by
// a call, both inside the continuation. This is the exact shape #259
// measured as NaN on main and called out-of-scope for that fix (it lives
// at this fix site, not #259's); confirm it's now resolved too.
const o6: any = { inner: { n: 3, m2(a: number) { return this.n + a; } } };
const r8 = o6?.inner.m2(4); // 7

// obj.method?.().b() - the receiver-less compileOptionalContinuation path
// used by compileOptionalCallExpression's own continuation sites. The
// call being continued here isn't itself optional, but the continuation
// after it goes through the same fixed code, so it benefits too.
const holder2: any = { m: () => ({ n: 5, get() { return this.n; } }) };
const r9 = holder2.m?.().get(); // 5

JSON.stringify([r1, r2, r3, r4, r5, r6, r7, r8, r9]);
// expect: [4,16,103,1003,10,15,null,7,5]
