// Test Readonly<T> assignment behavior
let obj = { x: 10, y: 20 };

// This should work - assigning to Readonly
let readonlyObj: Readonly<any> = obj;

// Try to read properties
console.log(readonlyObj.x);
console.log(readonlyObj.y);
readonlyObj.x; // Final expression

// Try to assign to readonly properties (should fail in the future)
// readonlyObj.x = 30; // TODO: Enable when readonly assignment checking is implemented

// expect: 10