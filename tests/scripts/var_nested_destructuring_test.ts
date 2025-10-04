// Test var with nested destructuring
// expect: 6

var [[a, b], c] = [[1, 2], 3];
a + b + c;
