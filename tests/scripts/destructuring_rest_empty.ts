// Test rest element with empty remainder
let first = 0;
let second = 0;
let rest = [];
[first, second, ...rest] = [1, 2];
rest.length;
// expect: 0