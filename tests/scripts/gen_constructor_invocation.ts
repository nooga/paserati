// no-typecheck
// expect_runtime_error: TypeError: Generator functions cannot be used as constructors
// Test: Generator functions should throw TypeError when called with new

var g = function*(){};
new g();
