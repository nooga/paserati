// expect: function,function,function,function,undefined,undefined,undefined,undefined,undefined,rec,function
// skip-typecheck
// paserati#542: a parenthesized named function expression used as a
// statement is an expression, not a function declaration - its name is not
// bound in the enclosing scope, and it is the statement's completion value.
const r = [];
r.push(typeof eval("(function ev(q) { return q; })"));
r.push(typeof eval("(function* g() {})"));
r.push(typeof eval("(async function af() {})"));
r.push(typeof eval("1; (function ev() {})"));
r.push(eval("(function ev() {}); typeof ev"));
(function leaked() {});
r.push(typeof leaked);
function outer() { (function inner() {}); return typeof inner; }
r.push(outer());
{ (function blk() {}); r.push(typeof blk); }
switch (1) { case 1: (function sw() {}); r.push(typeof sw); }
r.push(eval("(function f(n) { return n ? f(n - 1) : 'rec'; })")(3));
r.push(typeof hoisted);
function hoisted() {}
r.join();
