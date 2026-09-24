// A loop body is a Statement, not a Declaration.
// expect_compile_error: Lexical declaration cannot appear in a single-statement context

while (false) let x = 1;
