// no-typecheck
// Test TDZ (Temporal Dead Zone) for writing to uninitialized let/const
// expect_runtime_error: Cannot access variable before initialization
function outer() {
  function inner() { x = 10; }    // Attempt to write to x while in TDZ
  inner();                         // This throws
  let x = 5;
  return x;
}
outer();
