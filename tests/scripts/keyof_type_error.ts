// expect_compile_error: cannot assign type 'invalid' to variable 'invalidKey' of type

// Test keyof type checking with invalid assignments

type Person = { name: string; age: number };
type PersonKeys = keyof Person;

// This should fail - "invalid" is not a key of Person
let invalidKey: PersonKeys = "invalid";