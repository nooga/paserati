// expect_compile_error: Invalid regular expression
// \p{...} names are checked at parse time: an unknown property is an early error.
let re = /\p{Script=NotAScript}/u;
