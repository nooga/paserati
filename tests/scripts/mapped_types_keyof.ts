// expect: keyof mapping works

// Test mapped types with keyof operator

type Person = { name: string; age: number };

// Map all properties to string using keyof
type StringifiedPerson = { [P in keyof Person]: string };

"keyof mapping works";