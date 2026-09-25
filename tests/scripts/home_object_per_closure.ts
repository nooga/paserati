// [[HomeObject]] and a class constructor's [[Prototype]] belong to each
// closure, not to the function template shared by every evaluation (#570).
// no-typecheck
// expect: 1,2|3,4|b1!,b2!|s1?,s2?|true,true|true|true,true
function mk(p) { return { __proto__: p, m() { return super.x; } }; }
function mkc(p) { return { __proto__: p, get g() { return super.x; }, arr() { return (() => super.x)(); } }; }
function klass(base) { return class extends base { m() { return super.m() + "!"; } static s() { return super.s() + "?"; } }; }
class B1 { m() { return "b1"; } static s() { return "s1"; } }
class B2 { m() { return "b2"; } static s() { return "s2"; } }
const K1 = klass(B1), K2 = klass(B2);
function mkf() { return function () {}; }
const f1 = mkf(), f2 = mkf();
Object.setPrototypeOf(f1, null);
[
  [mk({ x: 1 }).m(), mk({ x: 2 }).m()].join(","),
  [mkc({ x: 3 }).g, mkc({ x: 4 }).arr()].join(","),
  [new K1().m(), new K2().m()].join(","),
  [K1.s(), K2.s()].join(","),
  [Object.getPrototypeOf(K1) === B1, Object.getPrototypeOf(K2) === B2].join(","),
  String(Object.getPrototypeOf(f2) === Function.prototype),
  [B1.isPrototypeOf(K1), B2.isPrototypeOf(K2)].join(","),
].join("|");
