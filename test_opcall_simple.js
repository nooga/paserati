var iter = {};
iter[Symbol.iterator] = function() {
  throw new Error("Iterator error");
};

function* f([x]) {}

try {
  f(iter);
  console.log("FAIL - should have thrown");
} catch (e) {
  console.log("SUCCESS - caught:", e.message);
}
