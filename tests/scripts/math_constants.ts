// Test Math constants
let pi = Math.PI;
let e = Math.E;
let ln2 = Math.LN2;
let ln10 = Math.LN10;
let log2e = Math.LOG2E;
let log10e = Math.LOG10E;
let sqrt2 = Math.SQRT2;
let sqrt1_2 = Math.SQRT1_2;

// Verify they are numbers and in reasonable ranges
let allValid =
  pi > 3.14 &&
  pi < 3.15 &&
  e > 2.7 &&
  e < 2.8 &&
  ln2 > 0.69 &&
  ln2 < 0.7 &&
  ln10 > 2.3 &&
  ln10 < 2.31 &&
  sqrt2 > 1.41 &&
  sqrt2 < 1.42;

// expect: true
allValid;
