// expect_compile_error: Cannot find name 'T'.

// Test mapped type with undefined constraint type

type Test = { [P in T]: number };

"error test";