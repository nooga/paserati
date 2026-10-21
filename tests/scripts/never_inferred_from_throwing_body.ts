// expect: ok
// A function expression, arrow or object literal method whose body cannot
// complete returns never; a declaration or class method returns void.
const o = { m() { throw new Error("x"); } };
const arrow = () => { throw new Error("y"); };
const expr = function () { throw new Error("z"); };
function decl() { throw new Error("w"); }
class C { m() { throw new Error("v"); } get g(): number { throw new Error("u"); } }

function tryNum(f: () => number): string {
  try { f(); return "no"; } catch (e) { return "ok"; }
}
const p = new Proxy({}, { has() { throw new Error("t"); } });
let r = tryNum(o.m) + tryNum(arrow) + tryNum(expr);
const d: void = decl === decl ? undefined : decl();
r === "okokok" ? "ok" : r;
