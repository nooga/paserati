// Test delete operator on Math constants (non-configurable properties)
// Type checker should reject delete on readonly properties
// expect_compile_error: The operand of a 'delete' operator cannot be a read-only property.

delete Math.E;
