// paserati#452 - NewArrayWithArgs (pkg/vm/value.go) used to implement the
// Array(...) *constructor*'s special "single numeric argument means a
// sparse array of that length" rule (ECMAScript 23.1.1.1), but several
// unrelated callers reused it just to build a literal array from an
// already-computed []Value slice - Array.prototype.slice and JSON.parse
// among them. Whenever such a result happened to be exactly one number,
// they silently got back a length-N hole array instead of a one-element
// array containing that number. `new Array(n)` for a lone numeric n is the
// ONLY construct that should behave this way.
// expect: true
// no-typecheck

function sameArray(a: any[], b: any[]): boolean {
  return a.length === b.length && a.every((v, i) => v === b[i]);
}

sameArray([1, 5].slice(1), [5]) &&
sameArray([5].slice(0, 1), [5]) &&
sameArray(Array.of(5), [5]) &&
sameArray(JSON.parse("[5]"), [5]) &&
new Array(3).length === 3 &&
!(0 in new Array(3)) && // new Array(n) sparse semantics still intact
sameArray(new Array(5, 6, 7), [5, 6, 7]);
