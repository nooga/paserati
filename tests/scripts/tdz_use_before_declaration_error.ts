// expect_compile_error: Block-scoped variable 'x' used before its declaration.
// A use that is not deferred into a function body, ahead of the let.

function f() {
  const y = x + 1;
  let x = 1;
  return y;
}
f();
