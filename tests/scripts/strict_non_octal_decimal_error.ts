// NonOctalDecimalIntegerLiterals (08, 09) are legacy syntax, rejected in
// strict mode just like legacy octal literals.
// expect_compile_error: Octal literals are not allowed in strict mode
"use strict";

let x = 08;
