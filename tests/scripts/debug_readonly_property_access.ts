// Test property access on Readonly<T>
let obj = { x: 10, y: 20 };
let readonlyObj: Readonly<any> = obj;

console.log(readonlyObj.x);
readonlyObj.x; // Final expression

// expect: 10