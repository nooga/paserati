// Test rest element length
let first = 0;
let rest = [];
[first, ...rest] = [1, 2, 3, 4, 5];
rest.length;
// expect: 4