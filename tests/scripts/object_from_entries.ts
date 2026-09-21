// expect: true
// skip-typecheck
// Object.fromEntries must create ENUMERABLE data properties and must consume
// any iterable (not just a real array). See #488: it previously used
// SetOwnNonEnumerable and only walked TypeArray, so the result was invisible
// to Object.keys/JSON.stringify/spread and empty for a Map or a generator.

let results: boolean[] = [];

const obj = Object.fromEntries([["a", 1], ["b", 2]]);
results.push(obj.a === 1 && obj.b === 2);
results.push(JSON.stringify(obj) === '{"a":1,"b":2}');
results.push(Object.keys(obj).join(",") === "a,b");
results.push(JSON.stringify({ ...obj }) === '{"a":1,"b":2}');

const desc = Object.getOwnPropertyDescriptor(obj, "a");
results.push(desc.value === 1 && desc.writable === true && desc.enumerable === true && desc.configurable === true);

let seen: string[] = [];
for (const k in obj) seen.push(k);
results.push(seen.join(",") === "a,b");

// Non-array iterables
results.push(JSON.stringify(Object.fromEntries(new Map([["x", 10], ["y", 20]]))) === '{"x":10,"y":20}');
function* gen() {
  yield ["g", 1];
  yield ["h", 2];
}
results.push(JSON.stringify(Object.fromEntries(gen())) === '{"g":1,"h":2}');

// Keys go through ToPropertyKey: symbols stay symbols, everything else stringifies
const sym = Symbol("s");
const o2 = Object.fromEntries([[sym, 5], [1, "one"]]);
results.push(o2[sym] === 5 && o2["1"] === "one" && Object.keys(o2).join(",") === "1");

results.push(JSON.stringify(Object.fromEntries([])) === "{}");

// Prototype is Object.prototype, not null
results.push(Object.getPrototypeOf(obj) === Object.prototype);

// Error cases
let threw = 0;
try {
  Object.fromEntries(null);
} catch (e) {
  threw++;
}
try {
  Object.fromEntries([1, 2]);
} catch (e) {
  threw++;
}
try {
  Object.fromEntries(5);
} catch (e) {
  threw++;
}
results.push(threw === 3);

results.every((r) => r === true);
