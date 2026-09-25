// Array.from builds its result with a constructor this value: subclasses
// and custom constructors, on both the iterable and array-like paths (#555).
// no-typecheck
// expect: true,true,1,2|4|6,2,b,boom:true
class S extends Array {}
function C() { this.k = 1; }
const c = Array.from.call(C, { length: 2, 0: "a", 1: "b" });
let closed = false;
const it = { [Symbol.iterator]() { return { next: () => ({ value: 1, done: false }), return() { closed = true; return {}; } }; } };
let err = "";
try { S.from(it, () => { throw new Error("boom"); }); } catch (e) { err = e.message + ":" + closed; }
[
  S.from([1]) instanceof S,
  S.from({ length: 1 }) instanceof S,
  Array.from.call(C, [1]).k,
  S.from([1, 2, 3], (x) => x * 2).join("|"),
  c.length,
  c[1],
  err,
].join(",");
