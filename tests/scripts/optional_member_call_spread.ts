// Optional member access (obj?.method(...)) whose continuation is a call
// with spread arguments must be compiled with spread-aware calling
// convention, not the fixed-register-block path (issue #256).
//
// This is the mirror image of optional_call_member_spread.ts (#188):
// here the `?.` is on the member access, and the trailing `(...)` is an
// ordinary call that becomes part of the same optional chain.

const args = [1, 2, 3];

// obj?.method(...args) - the core repro from #256.
// A real method (not an arrow) so `this` binding through the spread
// call path is actually exercised (this discriminates a wrong-register
// receiver from a merely-not-crashing call).
const o: any = {
  n: 7,
  m(...a: number[]) {
    return this.n + a.length;
  },
};
const r1 = o?.m(...args);

// obj?.["method"](...args) - same shape via computed/bracket access.
// (Uses an arrow, not a method: obj?.[expr](...) continuations don't
// thread a `this` receiver at all - a separate, pre-existing gap
// unrelated to spread arguments - so this isolates the spread handling.)
const o2: any = { m: (...a: number[]) => a.reduce((x, y) => x + y, 0) };
const r2 = o2?.["m"](...args);

// Mixed regular + spread arguments through the optional-access path.
const r3 = o?.m(10, ...args, 20);

// Nullish base must still short-circuit to undefined (per spec/Node),
// not crash, even though the continuation contains a spread call.
const n: any = null;
const r4 = n?.m(...args);

// a?.b.c(...args) - multi-level continuation (Member -> Call) ending in
// a spread call; only the leading access is optional.
const o3: any = { inner: { g: (...a: number[]) => a.reduce((x, y) => x + y, 0) } };
const r5 = o3?.inner.g(...[10, 20, 3]);

JSON.stringify([r1, r2, r3, r4, r5]);
// expect: [10,6,12,null,33]
