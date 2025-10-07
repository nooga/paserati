// Test var vs let scoping difference
// expect: 20

function test() {
  var x = 10;
  if (true) {
    var x = 20; // Same variable due to var function scoping
  }
  return x; // Should be 20 because var is function-scoped
}

test();
