// expect_compile_error: is not assignable to type 'string | number | symbol'.

// Test type error for invalid left operand
let obj = { name: "John" };
true in obj;