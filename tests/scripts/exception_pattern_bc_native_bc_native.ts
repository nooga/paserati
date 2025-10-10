// expect: true
// Test bc -> native -> bc -> native exception pattern
let caught = false;
function userCallback() {
  // This will call native function that should throw
  JSON.parse("{invalid json}");
}

try {
  // Find a builtin that takes a callback - let's use Array.map
  [1, 2, 3].map(function (x) {
    if (x === 2) {
      userCallback(); // bc -> native that throws
    }
    return x * 2;
  });
} catch (e) {
  console.log("Caught nested error:", e.message);
  caught = true;
}
console.log("caught", caught);
caught;
