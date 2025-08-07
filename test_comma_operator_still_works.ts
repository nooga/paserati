// Test that comma operator still works in expressions
let x = (1, 2, 3);
console.log(x); // Should be 3

let y = 0;
let z = (y = 5, y + 10);
console.log(y, z); // Should be 5, 15