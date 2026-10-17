// with objects belong to the frame and block that pushed them: they must not
// outlive return/throw/break/continue/catch, leak into callees, or stay
// pushed while a generator is suspended
// no-typecheck
// expect: global,global,global,global,global,global,global,global,in

var o = { x: "leak" };
var x = "global";
var out = [];

function ret() { with (o) { return 1; } }
ret();
with ({}) { out.push(x); }

function thr() { with (o) { throw 1; } }
try { thr(); } catch (e) {}
with ({}) { out.push(x); }

for (;;) { with (o) { break; } }
with ({}) { out.push(x); }

for (var i = 0; i < 2; i++) { with (o) { continue; } }
with ({}) { out.push(x); }

try { with (o) { throw 1; } } catch (e) { out.push(x); }

function callee() { with ({}) { return x; } }
with ({ x: "caller" }) { out.push(callee()); }

with (5) { out.push(x); }

var g = (function* () { with ({ x: "in" }) { yield 1; yield x; } })();
g.next();
with ({}) { out.push(x); }
out.push(g.next().value);

out.join(",");
