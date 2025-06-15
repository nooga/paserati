// Destructuring with default values test

// Array destructuring with defaults
let a;
let b;
[a = 10, b = 20] = [1];

// Test that first element gets actual value
a;
// expect: 1