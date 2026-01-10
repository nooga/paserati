// expect: 10
// no-typecheck
// Test valid backward reference in default parameters works

function test(x, y = x * 2) {
  return y;
}

test(5);
