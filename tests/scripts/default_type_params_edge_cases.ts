// Test edge cases for default type parameters

// Test constraint violation with default type
type BadDefault<T extends string = number> = T;
// This should fail at type checking because number doesn't extend string

// expect_compile_error: default type 'number' does not satisfy constraint 'string'