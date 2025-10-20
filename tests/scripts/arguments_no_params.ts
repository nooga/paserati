// Test arguments object with no named parameters
// no-typecheck
function test() {
  return arguments[0];
}
test(42);
// expect: 42
