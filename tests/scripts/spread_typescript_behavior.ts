// expect_compile_error: A spread argument must either have a tuple type or be passed to a rest parameter
// Test how TypeScript handles spread syntax in function calls

function sum(a: number, b: number, c: number): number {
  return a + b + c;
}

// Test 1: Array variable (should error - length unknown at compile time)
let numbers = [1, 2, 3];
sum(...numbers);

// Test 2: Direct array literal (should work in TypeScript)
sum(...[1, 2, 3]);

// Test 3: Another array variable (should also error)
let moreNumbers = [4, 5, 6];
sum(...moreNumbers);