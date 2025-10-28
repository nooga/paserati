// expect: done
// Test: yield as identifier in nested non-generator function inside generator

function* gen() {
  // This yield expression will pause execution
  // The nested function uses 'yield' as a variable name (which is allowed)
  return (function(arg) {
      var yield = arg + 1;
      return yield;
    }(yield));
}

var iter = gen();
// First next() - generator pauses at yield, returns undefined
var result = iter.next();
"done"; // Return value for test
