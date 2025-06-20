// Y combinator with explicit types
type RecursiveFunction<T> = (x: RecursiveFunction<T>) => T;

const Y = <T>(f: (g: T) => T): T => {
  const g = (x: RecursiveFunction<T>) => f(x(x));
  return g(g);
};

// Example usage with factorial
const factorial = Y<(n: number) => number>(
  (f) => (n) => n === 0 ? 1 : n * f(n - 1)
);

console.log(factorial(5)); // 120
