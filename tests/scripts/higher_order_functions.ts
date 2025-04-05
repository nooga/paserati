// expect: 14
// Test curried higher-order functions and function type annotations

// Define a type for a CURRIED function that takes a number and returns
// a function, which takes another number and returns a number.
type CurriedBinaryOperation = (x: number) => (y: number) => number;

// Define a higher-order function that applies a curried binary operation
function applyCurriedOperation(
  a: number,
  b: number,
  curriedOp: CurriedBinaryOperation
): number {
  // Apply arguments sequentially
  const intermediateFn = curriedOp(a);
  return intermediateFn(b);
}

// Define a curried subtract function using the 'function' keyword
function curriedSubtract(n1: number): (n2: number) => number {
  return function (n2: number): number {
    return n1 - n2;
  };
}

// Define a curried multiplier using an arrow function
const curriedMultiplier: CurriedBinaryOperation = (a: number) => (b: number) =>
  a * b;

// Assign the 'function' keyword curried function to a variable with the type annotation
let currentCurriedOp: CurriedBinaryOperation = curriedSubtract;

// Call the higher-order function with the curried 'subtract' function
let result1 = applyCurriedOperation(10, 3, currentCurriedOp); // result1 = curriedSubtract(10)(3) = 7

// Reassign the variable with the curried arrow function
currentCurriedOp = curriedMultiplier;

// Call the higher-order function with the curried 'multiplier' function
let result2 = applyCurriedOperation(result1, 2, currentCurriedOp); // result2 = curriedMultiplier(7)(2) = 14

// Return the first result for verification
result2;
