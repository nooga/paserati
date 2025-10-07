// Test 'in' operator with symbols
// expect: true

const sym = Symbol.toStringTag;
const obj = { [sym]: "test" };
sym in obj;
