// Test 'in' operator with well-known symbols
// expect: false

const obj = {};
Symbol.toStringTag in obj;
