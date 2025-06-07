// expect_compile_error: expected at least 3 arguments, but got 1

// ============================================================================
// Basic Spread Syntax Test - Shows Type Error
// ============================================================================

console.log("=== Basic Spread Syntax Test ===");

// Test with regular function (should show type error for spread)
// This shows that spread syntax is parsed but not properly handled by the type checker
function sum(a: number, b: number, c: number): number {
  return a + b + c;
}

let numbers = [1, 2, 3];
sum(...numbers);
