// expect_compile_error: Duplicate function implementation.

function f(): number {
  return 1;
}
function f(): number {
  return 2;
}
