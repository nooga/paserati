// expect: 1:TypeError 2:TypeError 3:TypeError 3:finally 4:TypeError 5:caught 6:TypeError map:TypeError gen:TypeError done end
// skip-typecheck
// paserati#517: a TypeError prepareCall throws itself (non-callable callee,
// class constructor without new, revoked proxy) at a tail call - return f()
// - skipped catch and silently halted the script, because the OpTailCall /
// OpTailCallMethod fallbacks never resynced frame state after it.
const out = [];
const o = { nope: 1 };
function f1() { return o.nope(); }
try { f1(); } catch (e) { out.push("1:" + e.constructor.name); }

function f2() { const q = {}; return q.nope(); }
try { f2(); } catch (e) { out.push("2:" + e.constructor.name); }

class K {}
function f3() { return K(); }
try { f3(); } catch (e) { out.push("3:" + e.constructor.name); } finally { out.push("3:finally"); }

const r = Proxy.revocable(function () {}, {});
r.revoke();
const rp = r.proxy;
function f4() { return rp(); }
try { f4(); } catch (e) { out.push("4:" + e.constructor.name); }

const nf = 5;
function f5() { return nf(); }
function g() { try { return f5(); } catch { return "5:caught"; } }
out.push(g());

function f6() { return o.nope(1, 2); }
function h() { return f6(); }
try { h(); } catch (e) { out.push("6:" + e.constructor.name); }

try { [1].map(() => o.nope()); } catch (e) { out.push("map:" + e.constructor.name); }
function* gen() { yield 1; return o.nope(); }
try { [...gen()]; } catch (e) { out.push("gen:" + e.constructor.name); }
function ok(n) { return n === 0 ? "done" : ok(n - 1); }
out.push(ok(10000));

out.push("end");
out.join(" ");
