// Test: Object destructuring in var statement should declare variables
// expect: 42

var { a: x } = { a: 42 };
x;
