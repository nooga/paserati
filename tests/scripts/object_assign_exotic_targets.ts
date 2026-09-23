// expect: u32:1 buf:1 ab:1 map:1 set:1 re:1 dv:1 date:1 promise:1 args:1 wm:1 ws:1 gen:1|u32:2 ab:2 map:2 re:2 dv:2 promise:2 args:2 wm:2|ab
// skip-typecheck
// paserati#528: Object.assign copied nothing when the target was a
// TypedArray, ArrayBuffer, Map, Set, RegExp, DataView, Promise, arguments, ...
// - setObjectAssignTargetProperty only handled plain objects, dicts, arrays
// and functions. Real rollup does `Object.assign(new Uint32Array(buf),
// { convertString })` and then calls buffer.convertString(...).
const targets = {
  u32: new Uint32Array(2), buf: new Uint8Array(2), ab: new ArrayBuffer(2), map: new Map(), set: new Set(),
  re: /x/, dv: new DataView(new ArrayBuffer(1)), date: new Date(0), promise: Promise.resolve(),
  args: (function () { return arguments; })(), wm: new WeakMap(), ws: new WeakSet(), gen: (function* () {})(),
};
const out = [];
for (const [k, t] of Object.entries(targets)) { Object.assign(t, { extra: 1 }); out.push(k + ":" + t.extra); }
const sk = Symbol("sk");
const out2 = [];
for (const k of ["u32", "ab", "map", "re", "dv", "promise", "args", "wm"]) { Object.assign(targets[k], { [sk]: 2 }); out2.push(k + ":" + targets[k][sk]); }

// rollup's getAstBuffer shape: the method lands on the typed array itself.
function getAstBuffer(astBuffer) {
  const array = new Uint32Array(astBuffer.buffer);
  const convertString = (pos) => String.fromCharCode(97 + array[pos]);
  return Object.assign(array, { convertString });
}
const buf = getAstBuffer(new Uint32Array([0, 1]));
[out.join(" "), out2.join(" "), buf.convertString(0) + buf.convertString(1)].join("|");
