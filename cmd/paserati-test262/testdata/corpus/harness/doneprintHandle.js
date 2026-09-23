// Minimal stand-in for test262's harness/doneprintHandle.js.
function $DONE(error) {
  if (error) {
    print("Test262:AsyncTestFailure:" + (error && error.name ? error.name + ": " + error.message : "Test262Error: " + String(error)));
  } else {
    print("Test262:AsyncTestComplete");
  }
}
