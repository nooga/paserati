// delete of an unqualified identifier is an early error in strict code.
// expect_compile_error: Delete of an unqualified identifier in strict mode
// no-typecheck
"use strict";

var x = 1;
delete x;
