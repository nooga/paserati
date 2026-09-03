// Test spread type error with a non-iterable value.
// Strings are iterable (Symbol.iterator) so [...str] is legal; numbers are not.
let notArray = 42;
[...notArray];
// expect_compile_error: spread syntax can only be applied to arrays