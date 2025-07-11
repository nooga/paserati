// var declarations with block scoping (TypeScript semantics)

{
    var x = "block";
}
// In our implementation, var has block scoping like let
x;
// expect_compile_error: undefined variable: x