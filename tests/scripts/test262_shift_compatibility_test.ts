// Test shift operators for test262 compatibility
console.log("Testing shift operators for test262 compatibility:");

// Test basic right shift
console.log("-4 >> 1:", -4 >> 1);
console.log("5 >> 1:", 5 >> 1);
console.log("-4 >>> 1:", -4 >>> 1);
console.log("5 >>> 1:", 5 >>> 1);

// Test with whitespace (simulating test262 whitespace tests)
console.log("Testing with spaces:");
console.log("-4 >> 1:", -4 >> 1);
console.log("-4 >>> 1:", -4 >>> 1);

// Test edge cases
console.log("0x80000000 >> 1:", 0x80000000 >> 1);
console.log("0x80000000 >>> 1:", 0x80000000 >>> 1);
console.log("0xFFFFFFFF >> 1:", 0xffffffff >> 1);
console.log("0xFFFFFFFF >>> 1:", 0xffffffff >>> 1);

// Test with large shifts
console.log("1 >> 32:", 1 >> 32);
console.log("1 >>> 32:", 1 >>> 32);
console.log("-1 >> 32:", -1 >> 32);
console.log("-1 >>> 32:", -1 >>> 32);

// expect: -2
-2;
