// expect_compile_error: cannot assign type 'number[]' to variable 'tuple' of type '[number, number, number]'
// Test spread with direct literals and tuple assignment

function sum(a: number, b: number, c: number): number {
  return a + b + c;
}

// Test direct array literal spread (should work)
sum(...[1, 2, 3]);

// Test with explicit tuple type (should work but currently fails due to tuple assignment issue)
let tuple: [number, number, number] = [1, 2, 3];
sum(...tuple);

console.log("Direct spread test");