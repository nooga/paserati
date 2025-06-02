// expect: 6765
// tests/bench/fib.ts
// Simple recursive Fibonacci for benchmarking

let fib = function (n) {
  if (n < 2) {
    return n;
  }
  return fib(n - 1) + fib(n - 2);
};

// Run the actual benchmark calculation
let result = fib(20);

result;
