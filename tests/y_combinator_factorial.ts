// The Y Combinator
const Y = (f) => ((x) => f((y) => x(x)(y)))((x) => f((y) => x(x)(y)));

// Factorial function generator
const FactGen = (f) => (n) => {
  if (n === 0) {
    return 1;
  } else {
    return n * f(n - 1);
  }
};

// Create the factorial function using the Y Combinator
const factorial = Y(FactGen);

// Calculate factorial of 5
factorial(5); // Should result in 120
