function *gen([x]) {
  yield x;
}

const iter = {};
iter[Symbol.iterator] = function() {
  console.log("iterator called");
  throw new Error("test error");
};

try {
  const g = gen(iter);
  console.log("FAIL - should have thrown");
} catch (e) {
  console.log("SUCCESS - caught:", e.message);
}
