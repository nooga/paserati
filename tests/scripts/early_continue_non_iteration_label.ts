// expect_compile_error: Illegal continue statement
label: {
  for (;;) {
    continue label;
  }
}
