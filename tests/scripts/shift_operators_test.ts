// Test shift operators
console.log("Testing right shift (>>):");

// Signed right shift - sign extension
console.log("5 >> 1:", 5 >> 1);
console.log("-5 >> 1:", -5 >> 1);
console.log("10 >> 2:", 10 >> 2);
console.log("-10 >> 2:", -10 >> 2);

console.log("Testing unsigned right shift (>>>):");

// Unsigned right shift - zero fill
console.log("5 >>> 1:", 5 >>> 1);
console.log("-5 >>> 1:", -5 >>> 1);
console.log("10 >>> 2:", 10 >>> 2);
console.log("-10 >>> 2:", -10 >>> 2);

console.log("Testing left shift (<<):");

// Left shift
console.log("5 << 1:", 5 << 1);
console.log("-5 << 1:", -5 << 1);
console.log("10 << 2:", 10 << 2);
console.log("-10 << 2:", -10 << 2);

// Test with larger numbers
console.log("Testing with larger numbers:");
console.log("0x80000000 >> 1:", 0x80000000 >> 1);
console.log("0x80000000 >>> 1:", 0x80000000 >>> 1);
console.log("0xFFFFFFFF >> 1:", 0xffffffff >> 1);
console.log("0xFFFFFFFF >>> 1:", 0xffffffff >>> 1);

// expect: 10
10;
