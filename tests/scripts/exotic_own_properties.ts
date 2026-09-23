// expect: dotSet: u32=1 ab=1 sab=1 dv=1 wm=1 ws=1 wr=1 fr=1 gen=1 agen=1 map=1 || idxSet: u32=1 ab=1 sab=1 dv=1 wm=1 ws=1 wr=1 fr=1 gen=1 agen=1 map=1 || symSet: u32=1 ab=1 sab=1 dv=1 wm=1 ws=1 wr=1 fr=1 gen=1 agen=1 map=1 || in: u32=true,false,true ab=true,false,true sab=true,false,true dv=true,false,true wm=true,false,true ws=true,false,true wr=true,false,true fr=true,false,true gen=true,false,true agen=true,false,true map=true,false,true || hasOwn: u32=true ab=true sab=true dv=true wm=true ws=true wr=true fr=true gen=true agen=true map=true || gopd: u32=1truetruetrue ab=1truetruetrue sab=1truetruetrue dv=1truetruetrue wm=1truetruetrue ws=1truetruetrue wr=1truetruetrue fr=1truetruetrue gen=1truetruetrue agen=1truetruetrue map=1truetruetrue || keys: u32=x|x|1|2 ab=x|x|1|2 sab=x|x|1|2 dv=x|x|1|2 wm=x|x|1|2 ws=x|x|1|2 wr=x|x|1|2 fr=x|x|1|2 gen=x|x|1|2 agen=x|x|1|2 map=x|x|1|2 || del: u32=true,undefined ab=true,undefined sab=true,undefined dv=true,undefined wm=true,undefined ws=true,undefined wr=true,undefined fr=true,undefined gen=true,undefined agen=true,undefined map=true,undefined || defProp: u32=5,true ab=5,true sab=5,true dv=5,true wm=5,true ws=5,true wr=5,true fr=5,true gen=5,true agen=5,true map=5,true || reflect: u32=true,3,true ab=true,3,true sab=true,3,true dv=true,3,true wm=true,3,true ws=true,3,true wr=true,3,true fr=true,3,true gen=true,3,true agen=true,3,true map=true,3,true || assignStr: u32=1 ab=1 sab=1 dv=1 wm=1 ws=1 wr=1 fr=1 gen=1 agen=1 map=1 || assignSym: u32=2 ab=2 sab=2 dv=2 wm=2 ws=2 wr=2 fr=2 gen=2 agen=2 map=2 || spreadSrc: u32=1,2 ab=1,2 sab=1,2 dv=1,2 wm=1,2 ws=1,2 wr=1,2 fr=1,2 gen=1,2 agen=1,2 map=1,2 || assignSrc: u32=1 ab=1 sab=1 dv=1 wm=1 ws=1 wr=1 fr=1 gen=1 agen=1 map=1 || forIn: u32=x ab=x sab=x dv=x wm=x ws=x wr=x fr=x gen=x agen=x map=x || freeze: u32=!TypeError ab=no-throw,1,true sab=no-throw,1,true dv=no-throw,1,true wm=no-throw,1,true ws=no-throw,1,true wr=no-throw,1,true fr=no-throw,1,true gen=no-throw,1,true agen=no-throw,1,true map=no-throw,1,true || noext: u32=no-throw,undefined,false ab=no-throw,undefined,false sab=no-throw,undefined,false dv=no-throw,undefined,false wm=no-throw,undefined,false ws=no-throw,undefined,false wr=no-throw,undefined,false fr=no-throw,undefined,false gen=no-throw,undefined,false agen=no-throw,undefined,false map=no-throw,undefined,false || protoSetter: u32=7,false ab=7,false sab=7,false dv=7,false wm=7,false ws=7,false wr=7,false fr=7,false gen=7,false agen=7,false map=7,false
// skip-typecheck
// paserati#528/#529: ordinary own properties on exotic kinds (TypedArray,
// ArrayBuffer, SharedArrayBuffer, DataView, WeakMap, WeakSet, WeakRef,
// FinalizationRegistry, generators, Map) across every property API. Before,
// `dv.x = 1` was dropped, `dv[sym] = 1` was a hard VM error, `in` threw, and
// keys/hasOwn/getOwnPropertyDescriptor/delete/Reflect/spread/assign/for-in
// ignored them. Values in each row match Node.
const mk = {
  u32: () => new Uint32Array(2), ab: () => new ArrayBuffer(2), sab: () => new SharedArrayBuffer(2),
  dv: () => new DataView(new ArrayBuffer(1)), wm: () => new WeakMap(), ws: () => new WeakSet(),
  wr: () => new WeakRef({}), fr: () => new FinalizationRegistry(() => {}),
  gen: () => (function* () {})(), agen: () => (async function* () {})(),
  map: () => new Map(),
};
const sym = Symbol("s");
const tests = {
  dotSet: (t) => { t.x = 1; return t.x; },
  idxSet: (t) => { t["x"] = 1; return t["x"]; },
  symSet: (t) => { t[sym] = 1; return t[sym]; },
  in: (t) => { t.x = 1; return ("x" in t) + "," + (sym in t) + "," + ("toString" in t); },
  hasOwn: (t) => { t.x = 1; return Object.hasOwn(t, "x"); },
  gopd: (t) => { t.x = 1; const d = Object.getOwnPropertyDescriptor(t, "x"); return d && d.value + "" + d.writable + d.enumerable + d.configurable; },
  keys: (t) => { t.x = 1; t[sym] = 2; return Object.keys(t).filter(k => k === "x").join() + "|" + Object.getOwnPropertyNames(t).filter(k => k === "x").join() + "|" + Object.getOwnPropertySymbols(t).length + "|" + Reflect.ownKeys(t).filter(k => k === "x" || k === sym).length; },
  del: (t) => { t.x = 1; const r = delete t.x; return r + "," + t.x; },
  defProp: (t) => { Object.defineProperty(t, "y", { value: 5, enumerable: true }); return t.y + "," + Object.keys(t).includes("y"); },
  reflect: (t) => Reflect.set(t, "z", 3) + "," + Reflect.get(t, "z") + "," + Reflect.has(t, "z"),
  assignStr: (t) => { Object.assign(t, { a: 1 }); return t.a; },
  assignSym: (t) => { Object.assign(t, { [sym]: 2 }); return t[sym]; },
  spreadSrc: (t) => { t.x = 1; t[sym] = 2; const o = { ...t }; return o.x + "," + o[sym]; },
  assignSrc: (t) => { t.x = 1; const o = Object.assign({}, t); return o.x; },
  forIn: (t) => { t.x = 1; const ks = []; for (const k in t) if (k === "x") ks.push(k); return ks.join(); },
  freeze: (t) => { t.x = 1; Object.freeze(t); let r; try { t.x = 2; r = "no-throw"; } catch (e) { r = e.constructor.name; } return r + "," + t.x + "," + Object.isFrozen(t); },
  noext: (t) => { Object.preventExtensions(t); let r; try { t.n = 2; r = "no-throw"; } catch (e) { r = e.constructor.name; } return r + "," + t.n + "," + Object.isExtensible(t); },
  protoSetter: (t) => { let seen; Object.defineProperty(Object.getPrototypeOf(t), "ps", { set(v) { seen = v; }, configurable: true }); t.ps = 7; const r = seen + "," + Object.hasOwn(t, "ps"); delete Object.getPrototypeOf(t).ps; return r; },
};
const rows = [];
for (const [tn, f] of Object.entries(tests)) {
  const cells = [];
  for (const [k, m] of Object.entries(mk)) {
    let r;
    try { r = String(f(m())); } catch (e) { r = "!" + e.constructor.name; }
    cells.push(k + "=" + r);
  }
  rows.push(tn + ": " + cells.join(" "));
}
rows.join(" || ");
