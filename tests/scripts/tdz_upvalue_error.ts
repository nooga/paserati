// no-typecheck
// Test TDZ (Temporal Dead Zone) for let/const with upvalues
// expect_runtime_error: Cannot access variable before initialization
function outer() {
  function inner() { return x; }  // x captured but in TDZ
  let result = inner();           // Accessing x before initialization throws
  let x = 5;
  return result;
}
outer();
