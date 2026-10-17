// expect: 1 number undefined function 33 5 2 12 true undefined 1
// no-typecheck
// paserati#72: a sloppy direct eval's var and function declarations are
// created in the calling function's variable environment: visible to that
// function (shadowing outer bindings) after the eval, but not outside it.

var results = [];

// var shadows an outer function's binding of the same name
function shadow() {
  var x = 0;
  function inner() { eval("var x = 1"); return x; }
  return inner();
}
results.push(shadow());

// the binding is the caller's, not a global
(function () { eval("var leaked590 = 1"); results.push(typeof leaked590); })();
results.push(typeof leaked590);

// function declarations, too, and they can be reassigned
(function () {
  eval("function fn() { return 33; }");
  results.push(typeof fn);
  results.push(fn());
  fn = 5;
  results.push(fn);
})();

// a compound assignment resolves its reference before the right-hand side
// creates a shadowing binding (test262 S11.13.2_A6)
var outerX;
var innerX = (function () {
  var x = 3;
  var r = (function () {
    x *= (eval("var x = 2;"), 4);
    return x;
  })();
  outerX = x;
  return r;
})();
results.push(innerX, outerX);

// eval-created bindings are deletable
results.push((function () { eval("var gone = 1"); return delete gone; })());

// an existing local is reused, not shadowed
results.push(String((function () { var keep; eval("var keep"); return keep; })()));
results.push((function () { var keep2 = 0; eval("var keep2 = 1"); return keep2; })());

results.join(" ");
