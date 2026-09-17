// Parking an object literal's own reference (and, for a computed key, its
// key register) in a spill slot while a nested container-literal property
// value compiles - the paserati#471 fix - must not change any observable
// evaluation order: a computed key is still evaluated before its value
// (ECMAScript requires this so a key with side effects runs first), and
// properties are still processed left to right, exactly as before the
// object's own reference was relocated out of a live register mid-compile.
// expect: k1,v1,k2,v2,k3,v3

let log: string[] = [];

function k(n: number): string {
  log.push("k" + n);
  return "p" + n;
}

function v(n: number, val: unknown): unknown {
  log.push("v" + n);
  return val;
}

let obj = {
  [k(1)]: v(1, { nested: { deep: 1 } }),
  [k(2)]: v(2, [1, [2, [3, 4]]]),
  [k(3)]: v(3, { a: { b: { c: 3 } } }),
};

log.join(",");
