// continue may only name a label that (through directly nested labels)
// labels an iteration statement.
// expect_compile_error: can only jump to a label of an enclosing iteration statement

label: {
  for (;;) {
    continue label;
  }
}
