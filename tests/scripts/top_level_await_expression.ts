// Test top-level await in expressions
// expect: 15

const p = Promise.resolve(10);
const result = (await p) + 5;
result;
