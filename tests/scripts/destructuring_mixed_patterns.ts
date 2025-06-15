// Test mixed destructuring: object containing array
let first = "";
let second = "";
let x = 0;
let y = 0;

{users: [first, second], coords: {x, y}} = {
    users: ["Alice", "Bob"],
    coords: {x: 10, y: 20}
};

let result = first + ":" + second + ":" + x + ":" + y;
result;
// expect: Alice:Bob:10:20