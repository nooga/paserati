// Test multiple var declarations in IIFE (like deepEqual.js)
// expect: 3

(function() {
  var EQUAL = 1;
  var NOT_EQUAL = -1;
  var UNKNOWN = 0;

  return EQUAL + NOT_EQUAL + UNKNOWN + 3;
})();
