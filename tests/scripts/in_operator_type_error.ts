// expect_compile_error: right-hand side of 'in' must be an object

// Test type error for non-object right operand
let x = "hello";
"prop" in x;