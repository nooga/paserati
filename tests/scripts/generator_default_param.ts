// Test generator with default parameter
function* gen(a = 1) {
  yield a;
}
var it = gen();
it.next().value;
// expect: 1
