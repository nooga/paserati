// expect_compile_error: unknown type name

// Test that infer parsing works

// Simple conditional type with infer
type TestInfer<T> = T extends infer U ? U : never;

"infer simple test";