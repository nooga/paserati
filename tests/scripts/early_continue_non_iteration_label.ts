// expect_compile_error: can only jump to a label of an enclosing iteration statement
label: {
  for (;;) {
    continue label;
  }
}
