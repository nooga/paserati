// expect_compile_error: 'await' is not a valid identifier here
// Class static blocks reserve await even outside async functions.
class C {
  static {
    var await = 1;
  }
}
