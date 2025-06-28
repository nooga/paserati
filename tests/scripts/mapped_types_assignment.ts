// expect: mapped type assignments work

// Test that mapped types actually work with assignments

type Person = { name: string; age: number };

// Test Partial<T> equivalent
type PartialPerson = { [P in keyof Person]?: Person[P] };

// Should be able to assign objects with missing properties  
let partial1: PartialPerson = {}; // empty object should work
let partial2: PartialPerson = { name: "Alice" }; // missing age should work
let partial3: PartialPerson = { age: 30 }; // missing name should work
let partial4: PartialPerson = { name: "Bob", age: 25 }; // all properties should work

// Test Readonly<T> equivalent with all properties as string
type StringifiedPerson = { [P in keyof Person]: string };

// Should only accept string values for all properties
let stringified: StringifiedPerson = { name: "Alice", age: "25" }; // string age should work

"mapped type assignments work";