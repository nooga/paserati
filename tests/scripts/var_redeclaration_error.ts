// var redeclaration should produce error

var x = 1;
var x = 2;
// expect_compile_error: identifier 'x' already declared