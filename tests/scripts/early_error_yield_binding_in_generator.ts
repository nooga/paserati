// expect_compile_error: 'yield' is not a valid identifier in a generator
function* g() {
  var yi\u0065ld = 1;
}
