// no-typecheck
// expect_runtime_error: TypeError
// Test: Object destructuring with undefined should throw TypeError

function f({}) {}
f(undefined);
