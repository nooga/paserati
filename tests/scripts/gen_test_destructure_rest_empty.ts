// Test generator with destructuring rest pattern containing empty array
// The destructuring [...[]] should consume the iterator
// expect: 1
var iterations = 0;
var iter = function*() {
  iterations += 1;
}();

var f = function*([...[]]) {
  // By the time we get here, iterations should be 1 because
  // the destructuring [...[]] should have consumed the iterator
};

f(iter).next();
iterations;
