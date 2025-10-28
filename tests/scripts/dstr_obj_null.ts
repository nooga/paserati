// no-typecheck
// expect_runtime_error: TypeError
// Test: Object destructuring with null should throw TypeError

function f({}) {}
f(null);
