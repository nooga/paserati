// Same as var_function_hoist_bare_block.ts but at module top level, matching
// the exact shape real @babel/preset-env code hit (#476).
// expect: 30

{
  var f = function (a: number, b: number) { return a + b; };
}

f(10, 20);
