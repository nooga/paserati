// Test unsigned right shift operator (>>>)
console.log("Testing unsigned right shift (>>>):");

// Basic tests
console.log("5 >>> 1:", 5 >>> 1);
console.log("-5 >>> 1:", -5 >>> 1);
console.log("10 >>> 2:", 10 >>> 2);
console.log("-10 >>> 2:", -10 >>> 2);

// Edge cases
console.log("0 >>> 1:", 0 >>> 1);
console.log("1 >>> 1:", 1 >>> 1);
console.log("-1 >>> 1:", -1 >>> 1);
console.log("0x80000000 >>> 1:", 0x80000000 >>> 1);
console.log("0xFFFFFFFF >>> 1:", 0xffffffff >>> 1);

// Test with larger shifts
console.log("0x80000000 >>> 32:", 0x80000000 >>> 32);
console.log("-1 >>> 32:", -1 >>> 32);

// expect: 0
0;
