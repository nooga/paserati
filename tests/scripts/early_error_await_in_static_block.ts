// expect_compile_error: 'await' is a reserved word that cannot be used here
// Class static blocks reserve await even outside async functions.
class C {
  static {
    var await = 1;
  }
}
