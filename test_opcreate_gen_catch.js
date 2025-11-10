// Test OpCreateGenerator exception catching

var iter = {};
iter[Symbol.iterator] = function() {
  console.log("Symbol.iterator called, throwing...");
  throw new Error("Iterator error");
};

function *gen([x]) {
  yield x;
}

try {
  console.log("About to call gen(iter)");
  const g = gen(iter);
  console.log("FAIL - should have thrown");
} catch (e) {
  console.log("SUCCESS - caught:", e.message);
}
