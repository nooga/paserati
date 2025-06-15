// Test basic rest elements in destructuring assignment
let first = 0;
let rest = [];
[first, ...rest] = [1, 2, 3, 4, 5];
rest[0];
// expect: 2