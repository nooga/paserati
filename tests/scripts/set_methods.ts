// Test Set methods (delete, clear, add chaining)
// expect: success

let mySet = new Set();

// Add values
mySet.add("a").add("b").add("c");

// Test delete
let deleteResult1 = mySet.delete("b");  // should return true
let deleteResult2 = mySet.delete("nonexistent");  // should return false

// Check state after delete
let hasA = mySet.has("a");
let hasB = mySet.has("b");
let hasC = mySet.has("c");
let sizeAfterDelete = mySet.size;

// Test clear
mySet.clear();
let sizeAfterClear = mySet.size;
let hasAAfterClear = mySet.has("a");

// Simple verification
(deleteResult1 === true && deleteResult2 === false && 
 hasA === true && hasB === false && hasC === true && sizeAfterDelete === 2 &&
 sizeAfterClear === 0 && hasAAfterClear === false) ? "success" : "failed"