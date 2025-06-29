// Test number method access
// expect_compile_error: property access is not supported on type number
let num = 42;
let result = num.toLocaleString();
console.log(result);