// Test that we correctly distinguish between block statements and destructuring

// Destructuring with declaration works
let {a, b} = {a: 10, b: 20};
a + b;
// expect: 30