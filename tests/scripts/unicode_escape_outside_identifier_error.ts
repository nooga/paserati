// A \u escape is only allowed where it denotes an identifier character; it
// cannot spell whitespace or punctuators.
// expect_compile_error: Invalid character.

var\u0009x = 1;
