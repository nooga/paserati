// expect_compile_error: Unexpected reserved word 'break'
// A reserved word spelled with \u escapes is still a reserved word, so it
// cannot be used as a binding name (it stays fine as a property name).
var \u0062reak = 1;
