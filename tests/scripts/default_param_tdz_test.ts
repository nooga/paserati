// expect_runtime_error: Cannot access 'y' before initialization
// no-typecheck
// Test forward reference in default parameters throws ReferenceError

function test(x = y, y) {
  return x;
}

test();
