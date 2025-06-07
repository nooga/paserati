// Test JSON.parse with objects and arrays
let objJson = '{"name":"Alice","age":30}';
let obj = JSON.parse(objJson);

let arrJson = "[1,2,3]";
let arr = JSON.parse(arrJson);

let nestedJson = '{"users":[{"name":"Bob"}],"count":1}';
let nested = JSON.parse(nestedJson);

// Check parsed values
let objCorrect = obj.name === "Alice" && obj.age === 30;
let arrCorrect = arr[0] === 1 && arr[1] === 2 && arr[2] === 3;
let nestedCorrect = nested.users[0].name === "Bob" && nested.count === 1;

// expect: true
objCorrect && arrCorrect && nestedCorrect;
