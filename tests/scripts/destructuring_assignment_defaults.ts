// expect: 10-5
// Test array destructuring assignment with defaults
let a: number, b: number;
let vals = [10];
[a, b = 5] = vals;
a + "-" + b;
