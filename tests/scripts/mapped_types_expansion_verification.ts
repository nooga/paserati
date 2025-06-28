// expect: expansion verification works

// Test that mapped types properly expand and allow correct assignments

type Person = { name: string; age: number };

// This should expand to { name?: string; age?: number }
type PartialPerson = { [P in keyof Person]?: Person[P] };

// Test assignments that should work with proper expansion
let test1: PartialPerson = {}; // empty object (all optional)
let test2: PartialPerson = { name: "Alice" }; // partial object
let test3: PartialPerson = { name: "Bob", age: 30 }; // full object

// This should expand to { name: string; age: string }
type StringifiedPerson = { [P in keyof Person]: string };

// Test assignment that should work
let test4: StringifiedPerson = { name: "Charlie", age: "25" }; // all strings

"expansion verification works";