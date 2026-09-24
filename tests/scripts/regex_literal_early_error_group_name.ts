// expect_compile_error: Invalid regular expression
// Two groups with the same name that can both participate are an early error.
let re = /(?<a>x)(?<a>y)/;
