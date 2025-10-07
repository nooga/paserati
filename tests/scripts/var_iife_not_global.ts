// Test that var in IIFE doesn't leak to global scope
// expect_runtime_error: is not defined

(function() {
  var x = 42;
})();

x; // Should be compile error - x not defined
