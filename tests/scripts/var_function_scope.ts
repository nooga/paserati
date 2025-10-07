// Test var is function-scoped, not block-scoped
// expect: 42

function test() {
  if (true) {
    var x = 42;
  }
  return x; // Should be accessible due to var hoisting
}

test();
