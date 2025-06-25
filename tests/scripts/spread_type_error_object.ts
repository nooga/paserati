// Test spread type error with non-object
let notObject = 42;
({...notObject});
// expect_compile_error: spread syntax requires an object