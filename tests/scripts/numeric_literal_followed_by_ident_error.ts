// The source character right after a numeric literal must not start an
// identifier.
// expect_compile_error: An identifier or keyword cannot immediately follow a numeric literal

let x = 3in [];
