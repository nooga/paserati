// Test Math.random function
let r1 = Math.random();
let r2 = Math.random();
let r3 = Math.random();

// Random values should be between 0 (inclusive) and 1 (exclusive)
let inRange = r1 >= 0 && r1 < 1 && r2 >= 0 && r2 < 1 && r3 >= 0 && r3 < 1;

// Random values should be different (very high probability)
let different = r1 !== r2 && r2 !== r3 && r1 !== r3;

// expect: true
inRange && different;
