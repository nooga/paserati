// Test shorthand methods with default and optional parameters
// expect: hello world!

const calculator = {
  // Default parameter
  square(x: number = 5): number {
    return x * x;
  },

  // Default parameter with expression
  add(a: number, b: number = 10): number {
    return a + b;
  },

  // Optional parameter (explicit ?)
  multiply(x: number, y?: number): number {
    return x * (y || 2);
  },

  // Mixed: optional and default
  greet(name: string = "world", punctuation?: string): string {
    return "hello " + name + (punctuation || "!");
  },
};

// Test default parameter - should use 5 as default: 25
console.log(calculator.square());

// Test default parameter - should use 10 as default for b: 15
console.log(calculator.add(5));

// Test optional parameter - should use 2 as fallback: 10
console.log(calculator.multiply(5));

// Test passing all parameters: 100
console.log(calculator.multiply(10, 10));

// Test default and optional together: "hello world!" (final result)
calculator.greet();
