// expect: ["function decl(a, b = 2, ...rest) { /* c */ return a; }","async function adecl() { await 1; }","function* gdecl() { yield 1; }","async function* agdecl() { yield 1; }","function named(x) { return x; }","async function (x) { return x; }","function* () {}","x => x","(x) => x","(x, y) => { return x + y; }","async x => x","async (x) => { await x; }","() => ({})","({ a }, [b]) => a + b","(a = 1) => a","m() { return 1; }","async am() {}","*gm() {}","async *agm() {}","get g() { return 1; }","set s(v) {}","[\"comp\" + \"uted\"]() {}","[Symbol.iterator]() {}","function () {}","() => 2","\"quoted key\"() {}","123() {}","class Base { constructor(x) { this.x = x; } }","class C extends Base {\n  #p = 1;\n  static s() {}\n  static async sa() {}\n  m() { return this.#p; }\n  get g() { return 1; }\n  set g(v) {}\n  *gen() {}\n  async am() {}\n  #priv() {}\n  static #spriv() {}\n  static { }\n  getPriv() { return this.#priv; }\n}","s() {}","async sa() {}","m() { return this.#p; }","get g() { return 1; }","set g(v) {}","*gen() {}","async am() {}","#priv() {}","class { }","class Named extends Base { m() {} }","m() {}","function anonymous(a,b\n) {\nreturn a + b\n}","function anonymous(\n) {\nreturn 1\n}","function anonymous(\n) {\n\n}","function () { [native code] }","function max() { [native code] }","function Function() { [native code] }","function call() { [native code] }","() => 'e'","function inner() { return 1; }",true,true,true,"async [\"ak\"]() {}","*[\"gk\"]() {}","get [\"gt\"]() { return 1; }","set [\"st\"](v) {}","async *[\"agk\"]() {}","async *sag() {}","[\"ck\"]() {}","[\"sck\"]() {}","async [\"ack\"]() {}","get [\"gck\"]() { return 1; }","get sg() { return 1; }"]
// skip-typecheck
// paserati#524: Function.prototype.toString returns an ECMAScript function's
// exact source text - declarations, expressions, arrows, async/generator
// forms, object and class methods (static/computed/private/accessors),
// classes, and the anonymous functions Function() creates - instead of the
// "function f() { [native code] }" form, which is for built-ins. Every
// entry matches Node.
const out = [];
const S = (f) => { try { return Function.prototype.toString.call(f); } catch (e) { return "!" + e.constructor.name; } };
function decl(a, b = 2, ...rest) { /* c */ return a; }
async function adecl() { await 1; }
function* gdecl() { yield 1; }
async function* agdecl() { yield 1; }
const fe = function named(x) { return x; };
const afe = async function (x) { return x; };
const gfe = function* () {};
const arrows = [x => x, (x) => x, (x, y) => { return x + y; }, async x => x, async (x) => { await x; }, () => ({}), ({ a }, [b]) => a + b, (a = 1) => a];
const o = {
  m() { return 1; },
  async am() {},
  *gm() {},
  async *agm() {},
  get g() { return 1; },
  set s(v) {},
  ["comp" + "uted"]() {},
  [Symbol.iterator]() {},
  prop: function () {},
  arrowProp: () => 2,
  "quoted key"() {},
  123() {},
};
class Base { constructor(x) { this.x = x; } }
class C extends Base {
  #p = 1;
  static s() {}
  static async sa() {}
  m() { return this.#p; }
  get g() { return 1; }
  set g(v) {}
  *gen() {}
  async am() {}
  #priv() {}
  static #spriv() {}
  static { }
  getPriv() { return this.#priv; }
}
const CE = class { };
const CE2 = class Named extends Base { m() {} };
out.push(...[decl, adecl, gdecl, agdecl, fe, afe, gfe].map(S));
out.push(...arrows.map(S));
out.push(S(o.m), S(o.am), S(o.gm), S(o.agm), S(Object.getOwnPropertyDescriptor(o, "g").get), S(Object.getOwnPropertyDescriptor(o, "s").set), S(o.computed), S(o[Symbol.iterator]), S(o.prop), S(o.arrowProp), S(o["quoted key"]), S(o[123]));
out.push(S(Base), S(C), S(C.s), S(C.sa), S(C.prototype.m), S(Object.getOwnPropertyDescriptor(C.prototype, "g").get), S(Object.getOwnPropertyDescriptor(C.prototype, "g").set), S(C.prototype.gen), S(C.prototype.am), S(new C(1).getPriv()), S(CE), S(CE2), S(CE2.prototype.m));
out.push(S(new Function("a", "b", "return a + b")), S(Function("return 1")), S(new Function()), S(decl.bind(null)), S(Math.max), S(class {}.constructor), S(function () {}.call));
out.push(S(eval("() => 'e'")));
const nested = function outer() { return function inner() { return 1; }; };
out.push(S(nested()));
out.push(String(decl) === S(decl), `${fe}` === S(fe), decl + "" === S(decl));
const o2 = { async ["ak"]() {}, *["gk"]() {}, get ["gt"]() { return 1; }, set ["st"](v) {}, async *["agk"]() {} };
class D { static async *sag() {} ["ck"]() {} static ["sck"]() {} async ["ack"]() {} get ["gck"]() { return 1; } static get sg() { return 1; } }
out.push(...[S(o2.ak), S(o2.gk), S(Object.getOwnPropertyDescriptor(o2, "gt").get), S(Object.getOwnPropertyDescriptor(o2, "st").set), S(o2.agk), S(D.sag), S(D.prototype.ck), S(D.sck), S(D.prototype.ack), S(Object.getOwnPropertyDescriptor(D.prototype, "gck").get), S(Object.getOwnPropertyDescriptor(D, "sg").get)]);
JSON.stringify(out);
