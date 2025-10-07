// expect_compile_error: left-hand side of 'in' must be of type 'string', 'number', or 'symbol'

// Test type error for invalid left operand
let obj = { name: "John" };
true in obj;