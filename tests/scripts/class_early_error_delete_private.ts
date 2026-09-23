// Deleting a private reference is an early SyntaxError, even parenthesized
class C {
  #x = 1;
  m() {
    return delete (this.#x);
  }
}

// expect_compile_error: Private fields can not be deleted
