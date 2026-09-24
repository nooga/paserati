// continue may only name a label that (through directly nested labels)
// labels an iteration statement.
// expect_compile_error: does not denote an iteration statement

label: {
  for (;;) {
    continue label;
  }
}
