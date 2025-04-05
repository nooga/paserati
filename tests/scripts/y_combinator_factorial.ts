// expect: 120
type YCombinatorRejectedMyApplicationOnce = (
  f: (rec: (arg: any) => any) => (arg: any) => any
) => (arg: any) => any;

// The Y Combinator
const Y: YCombinatorRejectedMyApplicationOnce = (f) =>
  ((x) => f((y) => x(x)(y)))((x) => f((y) => x(x)(y)));

// Factorial function generator
const FactGen = (f: (n: number) => number) => (n: number) => {
  if (n == 0) {
    return 1;
  }
  return n * f(n - 1);
};

// Create the factorial function using the Y Combinator
const factorial = Y(FactGen);

// Calculate factorial of 5
factorial(5); // Should result in 120
