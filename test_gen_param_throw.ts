// Test exception during parameter initialization

function throwError() {
  throw new Error("param error");
}

function *gen(x = throwError()) {
  yield x;
}

try {
  const g = gen();
  console.log("FAIL - should have thrown");
} catch (e) {
  console.log("SUCCESS:", e.message);
}
