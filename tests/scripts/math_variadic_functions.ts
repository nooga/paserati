// Test variadic Math functions
let max12345 = Math.max(1, 2, 3, 4, 5);
let min12345 = Math.min(1, 2, 3, 4, 5);
let maxEmpty = Math.max();
let minEmpty = Math.min();
let hypot34 = Math.hypot(3, 4); // Should be 5 (3-4-5 triangle)

// Test results
let variadicCorrect =
  max12345 === 5 &&
  min12345 === 1 &&
  maxEmpty === -Infinity &&
  minEmpty === Infinity &&
  hypot34 === 5;

// expect: true
variadicCorrect;
