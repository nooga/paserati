// expect_compile_error: Type '42' is not assignable to type 'string'.

// Test indexed access types with invalid assignment

type Person = { name: string; age: number };

// Access specific property and make invalid assignment
type PersonName = Person["name"]; // should be string
let personName: PersonName = 42;  // should error: number not assignable to string

"error test";