// A private name may only be declared once, except a getter/setter pair
class C {
  get #p() { return 1; }
  set #p(v) {}
  #m() {}
  #m = 2;
}

// expect_compile_error: Identifier '#m' has already been declared
