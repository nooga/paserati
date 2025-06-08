// Test contextual typing with spread syntax and tuples
// expect: 6

function sum(a: number, b: number, c: number): number {
  return a + b + c;
}

// Test 1: Direct array literal (should work in TypeScript due to contextual typing)
console.log(sum(...[1, 2, 3]));

// Test 2: Tuple type annotation
let tuple: [number, number, number] = [1, 2, 3];
console.log(sum(...tuple));

// Test 3: Const with explicit tuple type
const constTuple: [number, number, number] = [1, 2, 3];
console.log(sum(...constTuple));

// Return the first sum result
sum(...[1, 2, 3]);