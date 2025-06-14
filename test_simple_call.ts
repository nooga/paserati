// Test simple function call
function add(a: number, b: number) {
  console.log("add called with:", a, b);
  return a + b;
}

console.log("Direct call: add(2, 3) =", add(2, 3));

// Test .call()
console.log("Using call: add.call(null, 4, 5) =", add.call(null, 4, 5));