// Test arguments object via apply with spread
function test() {
  return arguments[0] + "," + arguments[1] + "," + arguments[2];
}
test.apply(null, [1, 2, 3, ...[]]);
// expect: 1,2,3
