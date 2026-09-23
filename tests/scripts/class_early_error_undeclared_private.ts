// A private name must be declared by an enclosing class (AllPrivateIdentifiersValid)
class Outer {
  #a = 1;
  m() {
    class Inner {
      f(o: any) { return o.#a + o.#b; }
    }
    return Inner;
  }
}

// expect_compile_error: Private field '#b' must be declared in an enclosing class
