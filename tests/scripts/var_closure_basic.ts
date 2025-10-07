// Test that var can be captured by nested functions
// expect: 1

function outer() {
  var x = 1;
  function inner() {
    return x;
  }
  return inner();
}

outer();
