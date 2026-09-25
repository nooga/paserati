// A throwing ToPropertyKey in `super[key] = v` reaches the enclosing catch,
// in the method and in its caller (#568).
// no-typecheck
// expect: inner:5,outer:6,k:7
const log = [];
const o = { m() { try { super[{ toString() { throw 5; } }] = 1; } catch (e) { log.push("inner:" + e); } } };
o.m();
const o2 = { m() { super[{ toString() { throw 6; } }] = 1; } };
try { o2.m(); } catch (e) { log.push("outer:" + e); }
const o3 = { m() { super[{ toString() { return "k"; } }] = 7; return this.k; } };
log.push("k:" + o3.m());
log.join(",");
