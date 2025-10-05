// Test basic top-level await
// expect: 42

const promise = Promise.resolve(42);
const result = await promise;
result;
