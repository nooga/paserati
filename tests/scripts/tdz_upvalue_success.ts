// Test TDZ (Temporal Dead Zone) for let/const with upvalues - successful case
// expect: 5
function outer() {
  let x = 5;                      // x initialized here
  function inner() { return x; }  // x captured after initialization
  return inner();                 // Access is OK
}
outer();
