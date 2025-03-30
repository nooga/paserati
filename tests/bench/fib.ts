// tests/bench/fib.ts
// Simple recursive Fibonacci for benchmarking

let fib = function (n) {
  if (n < 2) {
    // Need to implement if/else and < operator first
    return n;
  }
  return fib(n - 1) + fib(n - 2);
};

// Run the actual benchmark calculation
let result = fib(20);
result; // Expression statement, its value should be printed by VM

// Placeholder until if/else is implemented:
// let result = 1 + 2;
// result; // Return 3 for now
