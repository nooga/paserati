// expect_compile_error: cannot assign type '42' to variable 'personName' of type 'string'

// Test indexed access types with invalid assignment

type Person = { name: string; age: number };

// Access specific property and make invalid assignment
type PersonName = Person["name"]; // should be string
let personName: PersonName = 42;  // should error: number not assignable to string

"error test";