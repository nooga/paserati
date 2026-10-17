// expect: SyntaxError SyntaxError SyntaxError SyntaxError ok ok
// no-typecheck
// paserati#72: a sloppy direct eval's var (or function) declaration is a
// SyntaxError when a let/const/class of the same name lies between the eval
// call and the variable environment the var would hoist into.

function tryEval(f) {
  try { f(); return "ok"; } catch (e) { return e.constructor.name; }
}

const results = [
  // let in an enclosing block
  tryEval(function () { { let x; { eval("var x;"); } } }),
  // let in the function body itself
  tryEval(function () { let y; eval("var y;"); }),
  // function declaration, class binding
  tryEval(function () { class C {} eval("function C() {}"); }),
  // nested eval still crosses the outer function's let
  tryEval(function () { let z; eval("eval('var z;')"); }),
  // a let in a sibling block that has already closed is not in the way
  tryEval(function () { { let w; } eval("var w;"); }),
  // strict eval code keeps its vars to itself
  tryEval(function () { let v; eval("'use strict'; var v;"); }),
];
results.join(" ");
