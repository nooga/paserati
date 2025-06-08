// expect: undefined
// Test simple direct array literal spread (type checks but compiler not implemented)

function sum(a: number, b: number, c: number): number {
  return a + b + c;
}

// Test direct array literal spread (should work at type level)
sum(...[1, 2, 3]);

console.log("Simple spread test");
