// expect: keyof works

// Test keyof type with actual assignments and type checking

type Person = { name: string; age: number };
type PersonKeys = keyof Person; // Should resolve to keyof { name: string; age: number }

// This should work - keyof Person should include "name" and "age"
let validKey: PersonKeys = "name";
let anotherValidKey: PersonKeys = "age";

// Test index signatures with assignments
type StringDict = { [key: string]: string };
let dict: StringDict = { hello: "world", foo: "bar" };

"keyof works";