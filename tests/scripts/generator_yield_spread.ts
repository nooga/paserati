// Test generator with multiple spread+yield expressions
function* gen() {
  yield {
    ...yield yield,
    ...yield,
  }
}
var it = gen();
it.next();
"parsed";
// expect: parsed
