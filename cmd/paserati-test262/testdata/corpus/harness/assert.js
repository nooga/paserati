// Minimal stand-in for test262's harness/assert.js.
function assert(value, message) {
  if (value !== true) throw new Test262Error(message || "Expected true");
}
assert.sameValue = function (actual, expected, message) {
  if (!Object.is(actual, expected)) throw new Test262Error((message || "") + " Expected SameValue(" + String(actual) + ", " + String(expected) + ")");
};
