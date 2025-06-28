// expect: debug works

// Simple test to see if expansion is triggered
type Person = { name: string; age: number };

// This should trigger expansion
let partial: Partial<Person> = {};

"debug works";