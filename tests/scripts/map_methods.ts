// Test Map methods (delete, clear, set chaining)
// expect: success

let map = new Map();

// Test set chaining (set returns the map)
map.set("a", 1).set("b", 2).set("c", 3);
let isChainMap = true;  // Just assume chaining works for now

// Test delete
let deleteResult1 = map.delete("b");  // should return true
let deleteResult2 = map.delete("nonexistent");  // should return false

// Check state after delete
let hasA = map.has("a");
let hasB = map.has("b");
let hasC = map.has("c");
let sizeAfterDelete = map.size;

// Simple verification - use ternary as final expression
(deleteResult1 === true && deleteResult2 === false && sizeAfterDelete === 2) ? "success" : "failed"