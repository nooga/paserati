// let/const redeclaration should produce an error (var allows redeclaration)

let x = 1;
let x = 2;
// expect_compile_error: Identifier 'x' has already been declared
