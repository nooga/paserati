// Named function expression used for recursion
// expect: 120
let factorial = function f(n: number): number {
  if (n <= 1) return 1;
  return n * f(n - 1);
};
factorial(5);
