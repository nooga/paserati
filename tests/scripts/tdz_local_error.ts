// no-typecheck
// Test TDZ (Temporal Dead Zone) for let/const local variables
// expect_runtime_error: Cannot access variable before initialization
function test() {
  console.log(x);  // Accessing x before declaration - should throw
  let x = 5;
  return x;
}
test();
