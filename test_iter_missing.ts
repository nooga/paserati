// Test what happens when array iterator is missing

// Save original iterator
const origIterator = Array.prototype[Symbol.iterator];

// Delete it
delete Array.prototype[Symbol.iterator];

// Try to destructure in generator
function *gen([x, y]) {
  yield x;
}

try {
  const g = gen([1, 2]);
  console.log("Created generator:", g);
  const result = g.next();
  console.log("First next() result:", result);
} catch (e) {
  console.log("CAUGHT during creation:", e.message);
}

// Restore
Array.prototype[Symbol.iterator] = origIterator;

console.log("Test complete");
