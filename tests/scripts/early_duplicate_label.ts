// expect_compile_error: Duplicate label 'L'.
L: {
  L: for (;;) break L;
}
