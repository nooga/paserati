// expect: 1 2 6 2 11 7 k 1 1 ReferenceError
// no-typecheck
// paserati#72: direct eval code resolves names through the calling function's
// whole scope chain, including outer-function variables the caller's own
// source never mentions (and so would not otherwise capture).

function read() { var a = 1; function inner() { return eval("a"); } return inner(); }
function write() { var a = 1; function inner() { eval("a = 2"); } inner(); return a; }
function compound() { var a = 1; function inner() { eval("a += 5"); } inner(); return a; }
function update() { var a = 1; function inner() { eval("a++"); } inner(); return a; }
function destructure() { var a = 1; function inner() { eval("[a] = [a + 10]"); } inner(); return a; }
function forOf() { var a = 1; function inner() { eval("for (a of [7]) ;"); } inner(); return a; }
function forIn() { var a = 1; function inner() { eval("for (a in {k: 1}) ;"); } inner(); return a; }
function nested() { var a = 1; function inner() { return eval("eval('a')"); } return inner(); }
function closure() { var a = 1; function inner() { return eval("(function () { return a; })()"); } return inner(); }
function tdz() {
  function inner() { return eval("b"); }
  try { inner(); return "no error"; } catch (e) { return e.constructor.name; }
  let b = 1;
}

[read(), write(), compound(), update(), destructure(), forOf(), forIn(), nested(), closure(), tdz()].join(" ");
