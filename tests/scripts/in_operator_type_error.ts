// expect_compile_error: is not assignable to type 'object'.

// Test type error for non-object right operand
let x = "hello";
"prop" in x;