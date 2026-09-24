// expect: A-ok B-ok C | 1:1 2:2 3a:ReferenceError 3b:3 4a:ReferenceError 4b:ReferenceError,4 5:6 6:ab 7:ReferenceError 8a:ReferenceError 8b:m8 9:3210 10:Error,ReferenceError 11:{"o":1},ReferenceError 12:ReferenceError 13:true,2,13 14:field 15:15 16:16
// skip-typecheck
// paserati#520: an arrow resolves `this` through the enclosing environment
// when it RUNS. Arrows created in a derived constructor before super() used
// to snapshot the uninitialized sentinel and threw "Must call super
// constructor..." forever - tar's Unpack does `t.ondone = () => {...this...}`
// before super(t). Also: super() in an arrow nested inside another arrow
// called Function.prototype instead of the parent class.
class Base { constructor(opts) { this.opts = opts; } }
class A extends Base {
  constructor() {
    const later = () => this.tag;   // created before super()
    super({});
    this.tag = "A-ok";
    this.later = later;
  }
}
class B extends Base {
  constructor(t = {}) {
    t.ondone = () => { this.done = true; return "B-ok"; };
    super(t);
  }
}
class C extends Base {
  constructor(t = {}) {
    if ((t.ondone = () => this.constructor.name, super(t), false)) {}
  }
}
const head = [new A().later(), new B().opts.ondone(), new C().opts.ondone()].join(" ");
const out = [];
const t = (f) => { try { return String(f()); } catch (e) { return e.constructor.name; } };
class Base2 { constructor(x) { this.x = x; } }

// 1. nested arrows created before super
class N extends Base2 { constructor() { const outer = () => () => this.x; super(1); this.f = outer(); } }
out.push("1:" + new N().f());

// 2. arrow calling super(), then ctor reads this
class S extends Base2 { constructor() { const s = () => super(2); s(); out.push("2:" + this.x); } }
new S();

// 3. arrow created before super, called before super -> ReferenceError; after -> ok
class P extends Base2 { constructor() { const g = () => this.x; out.push("3a:" + t(g)); super(3); out.push("3b:" + g()); } }
new P();

// 4. super() twice: ctor then arrow -> ReferenceError; arrow then ctor -> ReferenceError
class D1 extends Base2 { constructor() { const s = () => super(4); super(4); out.push("4a:" + t(s)); } }
new D1();
class D2 extends Base2 { constructor() { const s = () => super(4); s(); out.push("4b:" + t(() => super(5)) + "," + this.x); } }
new D2();

// 5. super() inside a nested arrow created before super
class NS extends Base2 { constructor() { const a = () => () => super(6); a()(); out.push("5:" + this.x); } }
new NS();

// 6. cells are per-instance
class I extends Base2 { constructor(v) { const g = () => this.x; super(v); this.g = g; } }
const i1 = new I("a"), i2 = new I("b");
out.push("6:" + i1.g() + i2.g());

// 7. arrow passed to super, called by base ctor before this is bound -> ReferenceError
class CB { constructor(f) { this.r = t(f); } }
class CD extends CB { constructor() { super(() => this); } }
out.push("7:" + new CD().r);

// 8. super property in arrow before / after super()
class SPB { m() { return "m" + this.x; } }
class SP extends SPB { constructor() { const g = () => super.m(); out.push("8a:" + t(g)); super(); this.x = 8; out.push("8b:" + g()); } }
new SP();

// 9. recursion: derived ctor constructing another instance of itself inside
class R extends Base2 {
  constructor(n) {
    const g = () => this.x;
    if (n > 0) { const inner = new R(n - 1); super(n); this.inner = inner; } else super(n);
    this.g = g;
  }
}
const r = new R(3);
out.push("9:" + r.g() + r.inner.g() + r.inner.inner.g() + r.inner.inner.inner.g());

// 10. arrow escaping, ctor throws before super
let esc;
class E extends Base2 { constructor() { esc = () => this; throw new Error("boom"); } }
out.push("10:" + t(() => new E()) + "," + t(esc));

// 11. derived returning object; arrow still sees uninitialized this? (spec: this stays uninit) -> ReferenceError
let esc2;
class RO extends Base2 { constructor() { esc2 = () => this; return { o: 1 }; } }
out.push("11:" + JSON.stringify(new RO()) + "," + t(esc2));

// 12. arrow calling super after the constructor has returned -> ReferenceError
let lateSuper;
class LS extends Base2 { constructor() { lateSuper = () => super(12); super(1); } }
new LS();
out.push("12:" + t(lateSuper));

// 13. new.target and arguments inside arrow before super
class NT extends Base2 { constructor() { const g = () => [new.target === NT, arguments.length, this.x]; super(13); out.push("13:" + g().join()); } }
new NT(1, 2);

// 14. fields initialized after super, arrow reads field
class F extends Base2 { fld = "field"; constructor() { const g = () => this.fld; super(); out.push("14:" + g()); } }
new F();

// 15. tail-position arrow call
class TC extends Base2 { constructor() { const g = () => this.x; super(15); this.v = (() => g())(); } }
out.push("15:" + new TC().v);

// 16. Reflect.construct path
class RC extends Base2 { constructor() { const g = () => this.x; super(16); this.g = g; } }
out.push("16:" + Reflect.construct(RC, []).g());

head + " | " + out.join(" ");
