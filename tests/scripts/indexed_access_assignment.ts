// expect: assignments work

// Test indexed access types with actual assignments

type Person = { name: string; age: number };

// Access specific property and assign
type PersonName = Person["name"];
let personName: PersonName = "John"; // should work (string -> string)

type PersonAge = Person["age"];
let personAge: PersonAge = 30; // should work (number -> number)

// Test with keyof
type PersonValue = Person[keyof Person];
let personValue1: PersonValue = "Alice"; // should work (string is part of string | number)
let personValue2: PersonValue = 25;      // should work (number is part of string | number)

"assignments work";