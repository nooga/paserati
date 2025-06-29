// Test number toString method access
// expect_compile_error: property access is not supported on type number
let num = 42;
let result = num.toString();
console.log(result);