// expect_compile_error: non-simple parameter list
// A "use strict" directive is an early error in a function whose parameter
// list has defaults, destructuring or a rest parameter.
function f(a = 1) {
  "use strict";
  return a;
}
f();
