// expect_compile_error: function is not generic
function f(value: number) {
  return value;
}

f<string>(1);
