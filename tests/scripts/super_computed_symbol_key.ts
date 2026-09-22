// expect: sym|[1,"tag"] 5|get:b|7:b|deep|sym|STRING-KEY|undefined|static-sym|lit 1|wrote:x|TypeError
// skip-typecheck
// paserati#518: super[sym] used the symbol's string form "Symbol(desc)" as
// a string key, so reads found the wrong property (or undefined) and writes
// created a string-keyed one. minizlib's Symbol-named "protected" methods
// chain through exactly this.
const out = [];
const k = Symbol("k");
const acc = Symbol("acc");
const deepS = Symbol("deep");
const setLog = [];
class Root { [deepS]() { return "deep"; } }
class A extends Root {
  [k]() { return "sym"; }
  get [acc]() { return "get:" + this.tag; }
  set [acc](v) { setLog.push(v + ":" + this.tag); }
}
A.prototype["Symbol(k)"] = () => "STRING-KEY"; // decoy
class B extends A {
  constructor() { super(); this.tag = "b"; }
  m() { return super[k](); }
  s() { super[k] = 5; return [Object.getOwnPropertySymbols(this).length, Object.keys(this).join()]; }
  g() { return super[acc]; }
  a() { super[acc] = 7; }
  d() { return super[deepS](); }
  viaObj() { return super[{ [Symbol.toPrimitive]() { return k; } }](); }
  plain() { return super["Symbol(k)"](); }
  missing() { return super[Symbol("nope")]; }
}
const b = new B();
out.push(b.m());
out.push(JSON.stringify(b.s()) + " " + b[k]);
out.push(b.g());
b.a();
out.push(setLog.join());
out.push(b.d());
out.push(b.viaObj());
out.push(b.plain());
out.push(String(b.missing()));

const sk = Symbol("sk");
class SA { static [sk]() { return "static-sym"; } }
class SB extends SA { static t() { return super[sk](); } }
out.push(SB.t());

const base = { [k]() { return "lit"; } };
const lit = { __proto__: base, t() { return super[k](); }, u() { super[k] = 1; return Object.getOwnPropertySymbols(this).length; } };
out.push(lit.t() + " " + lit.u());

const _w = Symbol("_superWrite");
class Mini { write(d) { return "wrote:" + d; } }
class ZB extends Mini { [_w](d) { return super.write(d); } }
class Z extends ZB { [_w](d) { return super[_w](d); } write(d) { return this[_w](d); } }
out.push(new Z().write("x"));

const ro = Symbol("ro");
class RB {}
class RC extends RB {
  t() {
    Object.defineProperty(this, ro, { value: 1, writable: false });
    try { super[ro] = 2; return "no-throw"; } catch (e) { return e.constructor.name; }
  }
}
out.push(new RC().t());
out.join("|");
