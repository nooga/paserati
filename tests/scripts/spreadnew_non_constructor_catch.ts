// expect: caught
// Same as new_non_constructor_catch.ts but via OpSpreadNew (constructor call
// with spread arguments), which has its own separate "not a constructor"
// checks in op_spreadnew.go (issue #65).
let r = "not caught";
try {
  new Math(...[1]);
} catch (e) {
  r = "caught";
}
r;
