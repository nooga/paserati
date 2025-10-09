// Test arguments object with no named parameters
function test() {
  return arguments[0];
}
test(42);
// expect: 42
