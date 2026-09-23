// expect: 1,2,2~2,3~TypeError,TypeError,true,TypeError
// skip-typecheck
// paserati#535: a sloppy arguments object mapped its parameters as a raw
// slice of the frame's registers, so once the function returned and a later
// call reused them, arguments[0] read that call's values. Mapping is now
// through upvalues - closed on return, and shared with closures over the same
// parameter. Also: strict callee's %ThrowTypeError% accessor didn't throw.
const out = [];
const f = function (a, b) { return arguments; };
const x = f(1, 2);
function other(p, q, r, s) { const z = [p, q, r, s]; return z.length; }
other("P", "Q", "R", "S");
out.push([x[0], x[1], x.length].join());
function g(a) { const get = () => a, set = (v) => { a = v; }; return [arguments, get, set]; }
const [args, get, set] = g(1);
other(7, 8, 9, 10);
args[0] = 2; const r1 = get(); set(3);
out.push(r1 + "," + args[0]);
const t = (fn) => { try { return String(fn()); } catch (e) { return e.constructor.name; } };
const sa = (function () { "use strict"; return arguments; })(1);
const d = Object.getOwnPropertyDescriptor(sa, "callee");
out.push([t(() => sa.callee), t(() => { "use strict"; sa.callee = 1; }), d.get === d.set, t(() => d.get())].join());
out.join("~");
