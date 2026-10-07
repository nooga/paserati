// expect_compile_error: Type 'number' is not assignable to type 'string'.

// Test that mapped types properly reject invalid assignments

type Person = { name: string; age: number };

// Test StringifiedPerson that converts all properties to string
type StringifiedPerson = { [P in keyof Person]: string };

// This should error - age should be string, not number
let invalid: StringifiedPerson = { name: "Alice", age: 30 };

"error test";