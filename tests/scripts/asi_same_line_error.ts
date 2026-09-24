// Automatic semicolon insertion needs a line terminator, '}' or end of
// input before the offending token; two expressions on one line are an error.
// expect_compile_error: ';' expected.

let a = 1, b = 2;
a b;
