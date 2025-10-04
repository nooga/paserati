// Test nested destructuring with defaults
// expect: 3
let [[a, b] = [1, 2]] = [];
a + b;
