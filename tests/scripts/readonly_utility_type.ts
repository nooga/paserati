// Test Readonly<T> utility type
type ReadonlyPerson = Readonly<{ name: string; age: number }>;

let person: ReadonlyPerson = { name: "Alice", age: 25 };

// This should work (reading)
console.log(person.name);
person.name; // Final expression returns "Alice"
// expect: Alice
