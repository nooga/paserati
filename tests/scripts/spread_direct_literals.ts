// expect: undefined
// Test spread with direct literals and tuple assignment (now works with contextual typing!)

function sum(a: number, b: number, c: number): number {
  return a + b + c;
}

// Test direct array literal spread (should work)
sum(...[1, 2, 3]);

// Test with explicit tuple type (now works with contextual typing!)
let tuple: [number, number, number] = [1, 2, 3];
sum(...tuple);

console.log("Direct spread test");
undefined;