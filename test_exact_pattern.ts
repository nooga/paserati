// Exact pattern from failing test

var initCount = 0;
var iterCount = 0;
var iter = function*() { iterCount += 1; }();

try {
  const [[] = function() { initCount += 1; return iter; }()] = [];
  console.log("initCount:", initCount, "iterCount:", iterCount);
  if (initCount === 1 && iterCount === 0) {
    console.log("SUCCESS");
  } else {
    console.log("FAIL - wrong counts");
  }
} catch (e) {
  console.log("FAIL - caught exception:", e.message);
}
