// expect: 3|number[]|x|b|2|ok
// A union expected type must contextually type a literal through the one
// constituent it could be. Previously `[]` against `number[] | null` was
// checked without context as `unknown[]`, which the union then rejected
// (false TS2322) - also for `e.body || (e.body = [])`, the fallback idiom.
let e: { body: number[] | null; x: number } = { body: null, x: 0 };
e.body || (e.body = []);
e.body!.push(1, 2, 3);
let out: string[] = [String(e.body!.length)];

// Empty array literal against an array-or-null union, via initializer and assignment.
let b: number[] | null = [];
b = [];
b!.push(1);
out.push(typeof b![0] + "[]");

// Object literal against object-or-null union; its nested `[]` gets typed too.
let o: { a: string[] } | null = { a: [] };
o!.a.push("x");
out.push(o!.a[0]);

// Discriminated union: the literal-valued property picks the constituent.
type D = { k: "a"; v: number[] } | { k: "b"; v: string[] };
let d: D = { k: "b", v: [] };
out.push(d.k);

// Several array constituents: `[]` fits any of them.
let m: number[] | string[] = [];
m = [1, 2];
out.push(String(m.length));

// Arrow function against function-or-null union keeps contextual parameter typing.
let f: ((s: string) => string) | null = null;
f = (s) => s.toLowerCase();
if (f) out.push(f("OK"));

out.join("|");
