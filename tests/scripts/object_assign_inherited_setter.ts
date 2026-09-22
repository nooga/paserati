// expect: 4 _v|5 false|9 0|TypeError,TypeError,TypeError,TypeError,TypeError|x,y 1|{"a":3,"b":2} true|1,9 t|false true 2 false
// skip-typecheck
// paserati#521: Object.assign writes with a full ordinary [[Set]] - a setter
// inherited from the prototype (every class `set x()`) must run instead of
// being shadowed by a new own data property, and a failed [[Set]] throws.
const out = [];
class P { set v(x) { this._v = x * 2; } }
const p = new P();
Object.assign(p, { v: 2 });
out.push(p._v + " " + Object.keys(p).join());

class H {
  #t = "Unsupported";
  set type(v) { this.#t = v === "Directory" ? "5" : v; }
  key() { return this.#t; }
}
const h = new H();
Object.assign(h, { type: "Directory" });
out.push(h.key() + " " + Object.hasOwn(h, "type"));

const s = Symbol("s");
let seen;
const o = Object.create({ set [s](v) { seen = v; } });
Object.assign(o, { [s]: 9 });
out.push(seen + " " + Object.getOwnPropertySymbols(o).length);

function t(f) { try { f(); return "ok"; } catch (e) { return e.constructor.name; } }
out.push([
  t(() => Object.assign(Object.freeze({ a: 1 }), { a: 2 })),
  t(() => Object.assign(Object.create({ get g() { return 1; } }), { g: 2 })),
  t(() => Object.assign(Object.create(Object.freeze({ ro: 1 })), { ro: 2 })),
  t(() => Object.assign(Object.preventExtensions({}), { n: 1 })),
  t(() => Object.assign({ get a() { return 1; } }, { a: 2 })),
].join());

const log = [];
const px = new Proxy({}, { set(tg, k, v) { log.push(k); tg[k] = v; return true; } });
Object.assign(px, { x: 1, y: 2 });
out.push(log.join() + " " + px.x);

const plain = Object.assign({}, { a: 1 }, { b: 2, a: 3 });
out.push(JSON.stringify(plain) + " " + Object.getOwnPropertyDescriptor(plain, "a").enumerable);
const arr = Object.assign([1, 2], { 1: 9, tag: "t" });
out.push(arr.join() + " " + arr.tag);

// Reflect.set shares the receiver-side write: a new key on a
// non-extensible object fails.
const ne = Object.preventExtensions({ a: 1 });
out.push([Reflect.set(ne, "n", 1), Reflect.set(ne, "a", 2), ne.a, Object.hasOwn(ne, "n")].join(" "));
out.join("|");
