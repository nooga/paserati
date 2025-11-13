var iter = {};
iter[Symbol.iterator] = function() {
  throw new Error("Iterator error");
};

function* f([x]) {
  console.log("This should not print");
}

console.log("Before call");
try {
  f(iter);
  console.log("FAIL - should have thrown");
} catch (e) {
  console.log("SUCCESS - caught:", e.message);
}
console.log("After try-catch");
