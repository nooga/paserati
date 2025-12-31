// expect: 1
// Async function body should run synchronously until first await
var x = 0;
async function foo() {
  x = 1;  // This should run immediately, before foo() returns
}
foo();
x;  // Should be 1, not 0 (the async function body runs synchronously)
