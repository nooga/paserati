// expect: utility types work

// Demonstrate that we now have working utility types!

type Person = { name: string; age: number; email: string };

// Partial<T> equivalent - all properties optional
type PartialPerson = { [P in keyof Person]?: Person[P] };

// Test Partial functionality
let partial1: PartialPerson = {}; // All optional
let partial2: PartialPerson = { name: "Alice" }; // Partially filled
let partial3: PartialPerson = { name: "Bob", age: 30, email: "bob@example.com" }; // Fully filled

// Required<T> equivalent - all properties required (same as original)
type RequiredPerson = { [P in keyof Person]: Person[P] };

// Test Required functionality (should be same as Person)
let required: RequiredPerson = { name: "Charlie", age: 25, email: "charlie@example.com" };

// Pick<T, K> equivalent using union constraint
type PersonContact = { [P in "name" | "email"]: Person[P] };

// Test Pick functionality
let contact: PersonContact = { name: "David", email: "david@example.com" };

// Transform all properties to string (custom mapped type)
type StringifiedPerson = { [P in keyof Person]: string };

// Test custom transformation
let stringified: StringifiedPerson = { 
    name: "Eve", 
    age: "30", // age is now string
    email: "eve@example.com" 
};

"utility types work";