// expect: g:1,g:1 [6,false] | sg:SB,sg:SB,m:SB,3,false | 2,1,true,3 | TypeError | TypeError | TypeError | TypeError | q:7 1 | RangeError | a,3,t | hi cb! | arrow | undefined,undefined
// skip-typecheck
// super.x / super[key] (string keys) is base.[[Get]](key, this) and
// base.[[Set]](key, v, this): an accessor inherited from ABOVE the immediate
// super base (home object's [[Prototype]]) must run with this = receiver,
// and a failed [[Set]] throws in strict code. The opcodes used to check only
// the immediate base for accessors (and, for static methods, only the parent
// constructor itself), so inherited getters read undefined and inherited
// setters were shadowed by a data property on this.
const out = [];
class R { get g() { return "g:" + this.t; } set s(v) { this.log = v; } }
class A extends R {}
class B extends A {
  constructor() { super(); this.t = 1; }
  rg() { return super.g + "," + super["g"]; }
  ws() { super.s = 5; super["s"] = 6; return [this.log, Object.hasOwn(this, "s")]; }
}
const b = new B();
out.push(b.rg() + " " + JSON.stringify(b.ws()));

// static: inherited static getter/setter two levels up, this = receiver class
class SR { static get sg() { return "sg:" + this.name; } static set ss(v) { this.slog = v; } static m() { return "m:" + this.name; } }
class SA extends SR {}
class SB extends SA { static t() { super.ss = 3; return [super.sg, super["sg"], super.m(), this.slog, Object.hasOwn(this, "ss")].join(); } }
out.push(SB.t());

// super.x = v with no accessor: lands on this, not on the prototype
class DA { }
DA.prototype.d = 1;
class DB extends DA { w() { super.d = 2; super["e"] = 3; return [this.d, DA.prototype.d, Object.hasOwn(this, "d"), this.e].join(); } }
out.push(new DB().w());

// read-only inherited data property -> TypeError in class (strict) code
class RA {}
Object.defineProperty(RA.prototype, "ro", { value: 1, writable: false });
class RB extends RA { w() { try { super.ro = 2; return "no-throw"; } catch (e) { return e.constructor.name; } } }
out.push(new RB().w());

// getter-only inherited accessor -> TypeError
class GA { get only() { return 1; } }
class GB extends GA { w() { try { super.only = 2; return "no-throw"; } catch (e) { return e.constructor.name; } } }
out.push(new GB().w());

// non-extensible receiver
class NA {}
class NB extends NA { w() { Object.preventExtensions(this); try { super.x = 1; return "no-throw"; } catch (e) { return e.constructor.name; } } }
out.push(new NB().w());

// receiver's own accessor with a writable data prop on the chain -> [[Set]] fails
class OA {}
OA.prototype.p = 0;
class OB extends OA { w() { Object.defineProperty(this, "p", { get() { return 9; }, configurable: true }); try { super.p = 1; return "no-throw"; } catch (e) { return e.constructor.name; } } }
out.push(new OB().w());

// object literal super, sloppy: failures are silent
const proto = { get q() { return "q:" + this.z; } };
Object.defineProperty(proto, "fro", { value: 1, writable: false });
const lit = { __proto__: proto, z: 7, t() { super.fro = 5; return super.q + " " + this.fro; } };
out.push(lit.t());

// getter throws -> catchable
class TA { get boom() { throw new RangeError("x"); } }
class TB extends TA { w() { try { return super.boom; } catch (e) { return e.constructor.name; } } }
out.push(new TB().w());

// Array subclass receiver
class MyArr extends Array { w() { super[0] = "a"; super.length = 3; super.tag = "t"; return [this[0], this.length, this.tag].join(); } }
out.push(new MyArr().w());

// super.method() still binds this
class CA { hi() { return "hi " + this.n; } }
class CB extends CA { constructor() { super(); this.n = "cb"; } hi() { return super.hi() + "!"; } }
out.push(new CB().hi());

// arrow inside method
class AA { get v() { return this.k; } }
class AB extends AA { constructor() { super(); this.k = "arrow"; } f() { return (() => super.v)(); } }
out.push(new AB().f());

// missing property
class MA {}
class MB extends MA { f() { return super.nope + "," + super["nope"]; } }
out.push(new MB().f());
out.join(" | ");
