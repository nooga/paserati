// expect_compile_error: Expected 0 type arguments, but got 1.
function f(value: number) {
  return value;
}

f<string>(1);
