// expect_compile_error: rest element must be last element
// A trailing comma after a rest element is fine in an array literal but an
// early error once the literal is a destructuring pattern.
let x;
[...x,] = [1];
