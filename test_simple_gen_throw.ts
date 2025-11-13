// Simplest possible test

function *gen() {
  throw new Error("test");
}

try {
  const g = gen();
  console.log("Created:", g);
  g.next();
  console.log("FAIL");
} catch (e) {
  console.log("SUCCESS:", e.message);
}
