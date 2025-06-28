// expect_compile_error: unknown type name: T

// Test mapped type with undefined constraint type

type Test = { [P in T]: number };

"error test";