// Test arguments object via apply
function test() {
  return arguments[0];
}
test.apply(null, [42]);
// expect: 42
