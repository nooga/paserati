// expect_compile_error: Conversion of type 'string' to type 'number' may be a mistake

// Test invalid type assertion that should produce a compile error
let x: string = "hello";
let num = x as number;  // This should error
num;