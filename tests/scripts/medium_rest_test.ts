// Medium complexity test for rest parameters
// expect: Done

console.log("Starting medium test");

function sum(...numbers: number[]): number {
  let total = 0;
  for (let i = 0; i < numbers.length; i++) {
    total += numbers[i];
  }
  return total;
}

function multiply(...numbers: number[]): number {
  let result = 1;
  for (let i = 0; i < numbers.length; i++) {
    result *= numbers[i];
  }
  return result;
}

// Test multiple calls
console.log("sum(1, 2, 3):", sum(1, 2, 3));
console.log("sum(10, 20):", sum(10, 20));
console.log("multiply(2, 3, 4):", multiply(2, 3, 4));
console.log("multiply(5, 6):", multiply(5, 6));

// Test with some member access
let arr = [1, 2, 3, 4, 5];
console.log("array length:", arr.length);
console.log("sum of array:", sum(...arr));

("Done");
