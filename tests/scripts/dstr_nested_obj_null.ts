// no-typecheck
// expect_runtime_error: TypeError
// Test: Nested object destructuring with null should throw TypeError

function f([{ x }]) {}
f([null]);
