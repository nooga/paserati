// Deleting a private reference is an early SyntaxError, even parenthesized
class C {
  #x = 1;
  m() {
    return delete (this.#x);
  }
}

// expect_compile_error: The operand of a 'delete' operator cannot be a private identifier
