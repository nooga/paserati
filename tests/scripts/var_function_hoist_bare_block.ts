// Regression test for #476: `var f = function(){}` (or an arrow) declared
// inside a bare block must hoist its binding to the enclosing function/module
// scope like any other `var`, without panicking when the closure's register
// is finalized after the block's own scope has already been popped.
// expect: 30

function wrapper() {
  {
    var f = function (a: number, b: number) { return a + b; };
  }
  return f(10, 20);
}

wrapper();
