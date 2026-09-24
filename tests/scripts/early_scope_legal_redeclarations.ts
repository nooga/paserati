// expect: 2,2,4,5,6,e7,8,9,10
// skip-typecheck
// Declarations the scope early errors must keep accepting.

const out: any[] = [];

// A function body may repeat function declarations and mix them with var.
function body() {
  function f() { return 1; }
  function f() { return 2; }
  var f;
  return f();
}
out.push(body());

// Annex B.3.2.4: a sloppy block may repeat plain function declarations.
{
  function g() { return 1; }
  function g() { return 2; }
  out.push(g());
}

// A var in an inner function does not conflict with an outer let.
{
  let h = 4;
  (function () { var h = 3; })();
  out.push(h);
}

// Distinct blocks and loop heads each get their own scope.
{ let k = 1; }
{ let k = 5; out.push(k); }
for (let i = 6; i < 7; i++) { let j = i; out.push(j); }

// Annex B.3.4: a var may redeclare a plain catch parameter.
try { throw "e7"; } catch (e) { var e; out.push(e); }

// Annex B.3.3: a function declaration as the body of a sloppy if.
if (true) function viaIf() { return 8; }
out.push(viaIf());

// `let` followed by a line break still starts a declaration in a statement list.
let
  m = 9;
out.push(m);

// A labelled loop may be the target of continue through another label.
let n = 0;
outer: inner: for (let q = 0; q < 10; q++) { n++; continue outer; }
out.push(n);

out.join(",");
