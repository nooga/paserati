// A legacy octal escape in a directive becomes an error once a later
// "use strict" directive in the same prologue makes the function strict.
// expect_compile_error: Octal escape sequences are not allowed in strict mode

function f() {
  "\1";
  "use strict";
}
