// expect: true
// skip-typecheck
// Object(str) must build a String exotic object, i.e. one indexed character
// property per UTF-16 code unit plus its own "length" - exactly what
// new String(str) already produced. The Object constructor used to hand-roll
// the wrapper as NewObject(String.prototype) + [[PrimitiveValue]], so
// Object("ab")[0] was undefined and the wrapper had no length and no own keys.

let results: boolean[] = [];

const s = Object("ab");
results.push(typeof s === "object" && s instanceof String);
results.push(s[0] === "a" && s[1] === "b" && s[2] === undefined);
results.push(s.length === 2);
results.push(Object.keys(s).join(",") === "0,1");
results.push(String(s) === "ab" && s.valueOf() === "ab");
results.push(s.toUpperCase() === "AB");

// Index properties: enumerable, non-writable, non-configurable. length: not enumerable.
const d = Object.getOwnPropertyDescriptor(s, "0");
results.push(d.value === "a" && d.writable === false && d.enumerable === true && d.configurable === false);
const dl = Object.getOwnPropertyDescriptor(s, "length");
results.push(dl.value === 2 && dl.enumerable === false);

let seen: string[] = [];
for (const k in s) seen.push(k);
results.push(seen.join(",") === "0,1");

// Matches new String() and survives spread / iteration
results.push(Object.keys(Object("ab")).join(",") === Object.keys(new String("ab")).join(","));
results.push([...Object("ab")].join("") === "ab");

// Empty string has no index properties
const e = Object("");
results.push(e.length === 0 && Object.keys(e).length === 0);

// length counts UTF-16 code units, so an astral character takes two
const u = Object("a\u{1F600}");
results.push(u.length === 3 && u[0] === "a");

// The sibling primitive wrappers still box correctly
results.push(Object(42) instanceof Number && Object(42).valueOf() === 42);
results.push(Object(true) instanceof Boolean && Object(true).valueOf() === true);
const sym = Symbol("s");
results.push(Object(sym).valueOf() === sym);
results.push(Object(10n).valueOf() === 10n);

results.every((r) => r === true);
