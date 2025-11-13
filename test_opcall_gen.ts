// Test generator called via OpCall (not OpCreateGenerator)

function throwError() {
  throw new Error("param error");
}

// Use eval to force non-optimized path
const genCode = 'function *gen(x = throwError()) { yield x; }; gen';
const gen = eval(genCode);

try {
  const g = gen();
  console.log("FAIL - should have thrown");
} catch (e) {
  console.log("SUCCESS:", e.message);
}
