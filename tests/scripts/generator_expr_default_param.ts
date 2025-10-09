// Test generator expression with default parameter
var ref = function*(a, b = 39) {
  yield a;
  yield b;
};
var it = ref(42);
it.next().value;
// expect: 42
