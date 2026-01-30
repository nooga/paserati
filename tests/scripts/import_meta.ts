// expect_compile_error: 'import.meta' is only valid in module code
// Test that import.meta is correctly rejected in non-module (script) context
// Per ECMAScript spec, import.meta is only valid when the goal symbol is Module

let meta = import.meta;  // Should fail at compile time
