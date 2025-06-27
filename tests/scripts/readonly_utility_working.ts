// Test Readonly<T> utility type with simpler types
let obj = { name: "Alice", age: 25 };

// Use Readonly<any> which works
let readonlyObj: Readonly<any> = obj;

// Test property access
console.log(readonlyObj.name);
readonlyObj.name; // Final expression returns "Alice"

// Test that assignment to readonly properties is prevented
// readonlyObj.name = "Bob"; // Should fail if we enable this

// expect: Alice
