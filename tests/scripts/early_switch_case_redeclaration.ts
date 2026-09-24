// expect_compile_error: Identifier 'a' has already been declared
// All clauses of a switch share one scope.
switch (0) {
  case 1:
    let a = 1;
    break;
  default:
    let a = 2;
}
