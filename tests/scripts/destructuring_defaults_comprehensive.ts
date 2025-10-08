// Comprehensive destructuring defaults test

// Test array destructuring - second element gets default
let x;
let y;
[x = 10, y = 20] = [1];

// Second element should get default value
y;
// expect: 20