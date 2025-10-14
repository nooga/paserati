// Test how spread syntax works in function calls (JavaScript behavior)
// expect: 6

function sum(a: number, b: number, c: number): number {
  return a + b + c;
}

// Array variables can be spread (JavaScript allows this)
let numbers = [1, 2, 3];
sum(...numbers);