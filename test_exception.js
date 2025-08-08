function testThrow() {
  throw new Error("test error");
}

try {
  testThrow();
  console.log("Should not reach here");
} catch(e) {
  console.log("Caught:", e.message);
}

console.log("After try/catch");