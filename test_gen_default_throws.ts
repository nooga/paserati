// Test generator with default parameter that throws during initialization

function *gen([x = (function() { throw new Error("default error"); })()] ) {
  yield x;
}

try {
  const g = gen([undefined]);
  console.log("FAIL - should have thrown");
} catch (e) {
  console.log("SUCCESS - caught:", e.message);
}
