// Test generator called dynamically

function throwError() {
  throw new Error("param error");
}

function *genFunc(x = throwError()) {
  yield x;
}

// Call via indirect reference to avoid OpCreateGenerator optimization
const dynamicGen = genFunc;

try {
  const g = dynamicGen();
  console.log("FAIL - should have thrown");
} catch (e) {
  console.log("SUCCESS:", e.message);
}
