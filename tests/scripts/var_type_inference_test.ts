// Test that var destructuring properly infers types
// expect: 6

var [x, y, z] = [1, 2, 3];
// x, y, z should be inferred as numbers
let sum: number = x + y + z;
sum;
