// Test var inside IIFE - should be local to function, not global
// expect: 42

(function() {
  var x = 42;
  return x;
})();
