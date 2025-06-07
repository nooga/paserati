// Test JSON.stringify with objects and arrays
let obj = { name: "Alice", age: 30 };
let objJson = JSON.stringify(obj);

let arr = [1, 2, 3];
let arrJson = JSON.stringify(arr);

let nested = { users: [{ name: "Bob" }], count: 1 };
let nestedJson = JSON.stringify(nested);

// Check if JSON strings contain expected content
let objCorrect =
  objJson.includes('"name":"Alice"') && objJson.includes('"age":30');
let arrCorrect = arrJson === "[1,2,3]";
let nestedCorrect =
  nestedJson.includes('"users"') && nestedJson.includes('"name":"Bob"');

// expect: true
objCorrect && arrCorrect && nestedCorrect;
