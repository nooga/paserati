// expect_compile_error: ';' expected.
// ASI only inserts a semicolon before a line terminator, a closing brace or
// the end of input - two expression statements on one line need a semicolon.
let a = 1;
a = 2 a = 3;
