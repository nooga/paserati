// Exercises the generalized for-of fast path (BuiltinIterState kinds beyond
// plain array values): string iteration by code point incl. surrogate pairs
// and a lone surrogate, arguments objects, Array.prototype.keys/entries
// (incl. destructuring), and manual next() interleaved with for-of on a
// string iterator (shared index state).
// expect: a,b,𝄞,c|2|args:x,y,z|keys:0,1,2|0=p,1=q|mix:c
let out: any[] = [];

// String: "ab" + U+1D11E (surrogate pair) + "c"
let chars: any[] = [];
for (const ch of "ab\u{1D11E}c") chars.push(ch);
out.push(chars.join(","));

// Lone high surrogate iterates as a single unit
let lone = 0;
for (const ch of "\uD834x") lone++;
out.push(String(lone));

// Arguments object
function collect(...unused: any[]) {
  let got: any[] = [];
  // the checker doesn't type `arguments` as iterable; the runtime does
  const argsAny: any = arguments;
  for (const a of argsAny) got.push(a);
  return "args:" + got.join(",");
}
out.push(collect("x", "y", "z"));

// keys()
const arr: any[] = ["p", "q", "r"];
let ks: any[] = [];
for (const k of arr.keys()) ks.push(k);
out.push("keys:" + ks.join(","));

// entries() with destructuring
let es: any[] = [];
for (const [i, v] of ["p", "q"].entries()) es.push(i + "=" + v);
out.push(es.join(","));

// Manual next() shares state with for-of on a string iterator
const sit: any = "abc"[Symbol.iterator]();
sit.next(); // consume "a"
sit.next(); // consume "b"
let rest: any[] = [];
for (const ch of sit) rest.push(ch);
out.push("mix:" + rest.join(","));

out.join("|");
