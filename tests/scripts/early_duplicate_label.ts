// expect_compile_error: Label 'L' has already been declared
L: {
  L: for (;;) break L;
}
