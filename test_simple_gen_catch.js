// Test if try-catch can catch exceptions from generator calls

function *gen() {
  throw new Error("Generator error");
}

try {
  console.log("About to call generator");
  const g = gen();
  console.log("Generator created, calling .next()");
  g.next();
  console.log("FAIL - should have thrown");
} catch (e) {
  console.log("SUCCESS - caught:", e.message);
}
