// Test TDZ (Temporal Dead Zone) for local variables - successful case
// expect: 5
function test() {
  let x = 5;       // x is initialized here
  console.log(x);  // Access is OK - after initialization
  return x;
}
test();
