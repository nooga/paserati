// expect: [object WeakMap] [object WeakSet] [object WeakRef] [object FinalizationRegistry] [object DataView] [object WM:true] [object DataView] [object M] [object Set] [object Promise] [object AB] [object Changed] [object WeakMap] [object DataView]
// skip-typecheck
// paserati#523: Object.prototype.toString's @@toStringTag lookup had no case
// for WeakMap/WeakSet/WeakRef/FinalizationRegistry/DataView and reported
// "[object Object]". Unlisted kinds now resolve their real [[Prototype]],
// which also makes a subclass's own tag win for Map/ArrayBuffer (those
// cases used to read the intrinsic prototype directly).
const ts = (v) => Object.prototype.toString.call(v);
const out = [ts(new WeakMap()), ts(new WeakSet()), ts(new WeakRef({})),
  ts(new FinalizationRegistry(() => {})), ts(new DataView(new ArrayBuffer(1)))];
class WM extends WeakMap { get [Symbol.toStringTag]() { return "WM:" + (this instanceof WM); } }
class DV extends DataView {}
class M extends Map { get [Symbol.toStringTag]() { return "M"; } }
class S extends Set {}
class P extends Promise {}
class AB extends ArrayBuffer { get [Symbol.toStringTag]() { return "AB"; } }
out.push(ts(new WM()), ts(new DV(new ArrayBuffer(2))), ts(new M()), ts(new S()), ts(P.resolve()), ts(new AB(1)));
const w = new WeakSet();
Object.defineProperty(WeakSet.prototype, Symbol.toStringTag, { value: "Changed", configurable: true });
out.push(ts(w), String(new WeakMap()), `${new DataView(new ArrayBuffer(1))}`);
out.join(" ");
