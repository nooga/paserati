// expect: 77
// Comprehensive test of various language features

let globalVar = 10;
const PI = 3.14; // Roughly

// Function definition
function multiply(a, b) {
  return a * b;
}

// Arrow function & Closure
let makeCounter = function (start) {
  let count = start;
  return () => {
    // Arrow function closure
    count++;
    return count; // Using ++count semantics for return
  };
};

let counter1 = makeCounter(0);
let counter2 = makeCounter(100);

let result = 0;

// Basic ops and types
let flag = true;
let maybeNull = null;
let undef; // = undefined

result += 5 * 2; // 10
result -= 3; // 7
result /= 1; // 7
flag = !flag; // false

// If/else, comparison, logical ops
if ((flag && 1 < 0) || PI >= 3) {
  // false || true -> true
  result += multiply(2, 3); // result = 7 + 6 = 13
} else {
  result = 0; // Should not happen
}

// Loops, break, continue, compound assignment, increment/decrement
let loopSum = 0;
for (let i = 0; i < 10; i++) {
  if (i === 2) {
    continue; // Skip 2
  }
  if (i === 5) {
    break; // Stop at 5
  }
  loopSum += i; // 0 + 1 + 3 + 4 = 8
}
result += loopSum; // result = 13 + 8 = 21

let k = 5;
let whileVal = 1;
while (k-- > 0) {
  // k is 5, 4, 3, 2, 1 in condition; body runs 5 times
  whileVal *= 2;
}
// whileVal = 1 * 2 * 2 * 2 * 2 * 2 = 32
result += whileVal; // result = 21 + 32 = 53

let doCount = 0;
do {
  doCount++;
} while (doCount < 0); // Runs once
result += doCount; // result = 53 + 1 = 54

// Closures, ternary, nullish coalescing
result += counter1(); // c1=1, result = 54 + 1 = 55
result += counter1(); // c1=2, result = 55 + 2 = 57
result += counter2(); // c2=101, result = 57 + 101 = 158

let ternaryVal = counter1() > 5 ? 100 : 200; // c1=3, 3 > 5 is false, ternaryVal = 200
result -= ternaryVal; // result = 158 - 200 = -42

let final = (maybeNull ?? 50) + (undef ?? -8); // 50 + -8 = 42
result += final; // result = -42 + 42 = 0

// Update global
globalVar *= 7; // 70
globalVar += 7; // 77

result += globalVar; // result = 0 + 77 = 77

result; // Return final result
