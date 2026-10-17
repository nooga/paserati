// Each evaluation of a class declares distinct private names; private methods
// and accessors are installed before field initializers run; a proxy
// returned from a base constructor holds the derived class's private fields
// without going through its traps
// no-typecheck
// expect: ok,ok,TypeError,TypeError,42,42,3,method
function make() {
  class C {
    #m() { return "ok"; }
    get #g() { return 42; }
    a = this.#m();
    b = this.#g;
    call(o) { return o.#m(); }
  }
  return new C();
}
var c1 = make();
var c2 = make();
var out = [c1.call(c1), c2.call(c2)];
try { c1.call(c2); out.push("no error"); } catch (e) { out.push(e.constructor.name); }
try { c2.call(c1); out.push("no error"); } catch (e) { out.push(e.constructor.name); }
out.push(c1.b, c2.b);

var seen = [];
class ProxyBase {
  constructor() {
    return new Proxy(this, { get(t, p) { seen.push(String(p)); return t[p]; } });
  }
}
class Derived extends ProxyBase {
  #f = 3;
  method() { return this.#f; }
}
out.push(new Derived().method(), seen.join());
out.join(",");
