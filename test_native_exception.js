// Test that native function errors are caught as exceptions
function causeError() {
  // This will cause a native function error
  JSON.parse("{invalid json}");
}

try {
  causeError();
  console.log("Should not reach here");
} catch(e) {
  console.log("Caught error from native function:", e.message);
}

console.log("After try/catch");