// expect: property access works

// Test basic indexed access types

type Person = { name: string; age: number };

// Access specific property
type PersonName = Person["name"]; // should be string
type PersonAge = Person["age"];   // should be number

// Test with union of keys
type PersonNameOrAge = Person["name" | "age"]; // should be string | number

// Test with keyof
type PersonValue = Person[keyof Person]; // should be string | number

"property access works";