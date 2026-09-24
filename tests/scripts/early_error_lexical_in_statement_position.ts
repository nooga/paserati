// expect_compile_error: Lexical declaration cannot appear in a single-statement context
// The body of an if/loop is a Statement, so a let/const declaration there is an
// early error (only a braced block may hold one).
if (true) let x = 1;
