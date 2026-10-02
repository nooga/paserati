// A static method named "prototype" through a computed key throws a
// TypeError that an enclosing try/catch can catch; the script keeps running
// (#113).
// no-typecheck
// expect: caught:TypeError,fn:TypeError,finally,ok
const log: string[] = [];
try { class C { static ["prototype"]() {} } } catch (e) { log.push("caught:" + e.name); }
function f() { try { return class { static ["proto" + "type"]() {} }; } catch (e) { return "fn:" + e.name; } }
log.push(f());
try { class E { static ["prototype"]() {} } } catch (e) {} finally { log.push("finally"); }
const ok = class { static ["x" + 1]() { return 1; } };
log.push(ok.x1() === 1 ? "ok" : "bad");
log.join(",");
