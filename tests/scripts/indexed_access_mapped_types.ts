// expect: mapped types with indexed access work

// Test mapped types that use indexed access

type Person = { name: string; age: number };

// Create a mapped type that uses indexed access - this is the proper Partial<T> syntax
type PartialPerson = { [P in keyof Person]?: Person[P] };

// Create a mapped type that converts all properties to string
type StringifiedPerson = { [P in keyof Person]: string };

// Test indexed access directly
type PersonNameType = Person["name"]; // should be string
type PersonAgeType = Person["age"];   // should be number

"mapped types with indexed access work";