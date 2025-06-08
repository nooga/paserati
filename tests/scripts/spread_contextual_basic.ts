// Test basic contextual typing with spread syntax
// expect: 6

function sum(a: number, b: number, c: number): number {
  return a + b + c;
}

// Test 1: Direct array literal - TypeScript infers this as a tuple contextually
let result1 = sum(...[1, 2, 3]);

// Test 2: Explicitly typed tuple
let tuple: [number, number, number] = [1, 2, 3];
let result2 = sum(...tuple);

// Return the first result
result1;