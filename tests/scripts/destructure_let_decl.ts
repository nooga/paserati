// Test: Object destructuring with let should declare variables
// expect: 42

let { a: x } = { a: 42 };
x;
