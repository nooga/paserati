// Test const destructuring with empty array pattern

var initCount = 0;

try {
  const [[] = function() { initCount += 1; return []; }()] = [];
  console.log("Initialized, initCount:", initCount);
  console.log("SUCCESS - should have run initializer");
} catch (e) {
  console.log("FAIL - caught exception:", e.message);
}
