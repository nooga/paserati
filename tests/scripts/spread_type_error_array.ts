// Test spread type error with non-array
let notArray = "string";
[...notArray];
// expect_compile_error: spread syntax can only be applied to arrays