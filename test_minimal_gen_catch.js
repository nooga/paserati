// Minimal generator exception catching test

function *gen(x) {
  console.log("In generator, x =", x);
  yield 1;
}

try {
  console.log("About to call gen(null)");
  const g = gen(null);
  console.log("Generator created successfully");
} catch (e) {
  console.log("SUCCESS - caught:", e.message);
}

console.log("After try-catch");
