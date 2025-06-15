// Test nested array destructuring
let a = 0;
let b = 0;
let c = 0;
[a, [b, c]] = [1, [2, 3]];

let result = a + b + c;
result;
// expect: 6