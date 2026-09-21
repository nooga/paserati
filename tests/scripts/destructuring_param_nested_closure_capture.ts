// A parameter destructured through two levels of nesting (array pattern
// containing an object pattern) must be visible to a nested closure, not
// just to the outer function's own body. Regression test for #496.
function make([{a}]: [{a: number}]) {
  function inner() {
    return a;
  }
  return inner();
}

make([{a: 42}]);
// expect: 42
