// expect_compile_error: Type '42' is not assignable to type 'string' as required by index signature

// Test index signature type checking with invalid assignments

type StringDict = { [key: string]: string };

// This should fail - trying to assign number to string value
let dict: StringDict = { name: "valid", age: 42 };