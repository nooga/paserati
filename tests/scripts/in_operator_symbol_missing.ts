// Test 'in' operator with missing symbol property
// expect: false

const sym = Symbol("mySymbol");
const obj = { name: "test" };
sym in obj;
