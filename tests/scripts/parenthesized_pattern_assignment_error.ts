// A parenthesized object literal is not an assignment pattern.
// expect_compile_error: Invalid left-hand side in assignment
// no-typecheck

({}) = 1;
