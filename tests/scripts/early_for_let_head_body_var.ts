// expect_compile_error: Identifier 'x' has already been declared
for (let x of [1]) {
  var x = 2;
}
