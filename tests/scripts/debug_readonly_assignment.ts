// Debug readonly assignment step by step
let obj = { name: "Alice", age: 25 };
let readonlyAny: Readonly<any> = obj;  // This should work

console.log("Assignment to Readonly<any> works");
"Assignment to Readonly<any> works"; // Final expression

// expect: Assignment to Readonly<any> works