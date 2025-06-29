// Properly typed Y combinator implementation
// The Y combinator enables recursion without explicit self-reference

// The main Y combinator function
// We use 'any' for self-application parts where TypeScript's type system can't handle it,
// but keep the main interface strongly typed
const Y = <T extends (...args: any[]) => any>(f: (g: T) => T): T => {
  const g = (x: any) => f(((y: any) => x(x)(y)) as T);
  return g(g) as T;
};

// Alternative implementation that's more explicit about the mechanics
const Y_explicit = <T extends (...args: any[]) => any>(f: (g: T) => T): T => {
  const selfApply = (x: any) => (y: any) => x(x)(y);
  const g = (x: any) => f(selfApply(x) as T);
  return g(g) as T;
};

// Type definitions for our examples
type Factorial = (n: number) => number;
type Fibonacci = (n: number) => number;
type Sum = (n: number) => number;

// Test 1: Factorial function
const factorial = Y<Factorial>((f) => (n) => n === 0 ? 1 : n * f(n - 1));

// Test 2: Fibonacci function
const fibonacci = Y<Fibonacci>((f) => (n) => n <= 1 ? n : f(n - 1) + f(n - 2));

// Test 3: Sum from 1 to n
const sumToN = Y<Sum>((f) => (n) => n === 0 ? 0 : n + f(n - 1));

// Test 4: Using the explicit version
const factorial_explicit = Y_explicit<Factorial>(
  (f) => (n) => n === 0 ? 1 : n * f(n - 1)
);

// Run tests
console.log("=== Y Combinator Tests ===");
console.log("factorial(5):", factorial(5)); // Should be 120
console.log("factorial(0):", factorial(0)); // Should be 1
console.log("factorial(3):", factorial(3)); // Should be 6

console.log("fibonacci(10):", fibonacci(10)); // Should be 55
console.log("fibonacci(7):", fibonacci(7)); // Should be 13

console.log("sumToN(5):", sumToN(5)); // Should be 15 (1+2+3+4+5)
console.log("sumToN(10):", sumToN(10)); // Should be 55

console.log("factorial_explicit(4):", factorial_explicit(4)); // Should be 24

// Verify the Y combinator works correctly
console.log("=== Verification ===");
console.log("5! =", factorial(5), "(should be 120)");
console.log("Fib(10) =", fibonacci(10), "(should be 55)");
console.log("Sum(1..5) =", sumToN(5), "(should be 15)");
