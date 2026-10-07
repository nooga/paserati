// Test spread type error with non-object
let notObject = 42;
({...notObject});
// expect_compile_error: Spread types may only be created from object types.