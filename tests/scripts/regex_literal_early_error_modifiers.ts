// expect_compile_error: Invalid regular expression
// An invalid regular expression literal is an early error: the script is
// rejected when it is parsed, before any of it runs.
throw new Error("must not run");
let re = /(?i-i:a)/;
