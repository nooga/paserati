// Block scoping test - variables declared in block are not visible outside

{
    let blockVar = "inside";
}
// Trying to access blockVar should fail
blockVar;
// expect_compile_error: undefined variable: blockVar