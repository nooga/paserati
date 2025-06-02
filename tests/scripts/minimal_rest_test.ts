// Minimal test for rest parameters to isolate register exhaustion
console.log("Starting minimal test");

function sum(...numbers: number[]): number {
  let total = 0;
  for (let i = 0; i < numbers.length; i++) {
    total += numbers[i];
  }
  return total;
}

// Test one function call
console.log("sum(1, 2, 3):", sum(1, 2, 3));

("Done");

// expect: Done
