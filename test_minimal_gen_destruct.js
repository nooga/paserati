// Minimal generator destructuring exception test

function *gen([x]) {
  console.log("In generator, x =", x);
  yield x;
}

try {
  console.log("About to call gen(null)");
  const g = gen(null);
  console.log("FAIL - should have thrown");
} catch (e) {
  console.log("SUCCESS - caught:", e.message);
}

console.log("After try-catch");
