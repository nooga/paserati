// expect_compile_error: ']' expected.
// A rest element with an initializer in a method's array parameter is a
// syntax error, not a parser crash.
const obj = {
  method([...[x] = []]) {}
};
