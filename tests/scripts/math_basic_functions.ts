// Test basic Math functions
let abs1 = Math.abs(-5);
let abs2 = Math.abs(5);
let sqrt16 = Math.sqrt(16);
let pow23 = Math.pow(2, 3);
let floor32 = Math.floor(3.2);
let ceil32 = Math.ceil(3.2);
let round32 = Math.round(3.2);
let round37 = Math.round(3.7);

// Test results
let allCorrect =
  abs1 === 5 &&
  abs2 === 5 &&
  sqrt16 === 4 &&
  pow23 === 8 &&
  floor32 === 3 &&
  ceil32 === 4 &&
  round32 === 3 &&
  round37 === 4;

// expect: true
allCorrect;
