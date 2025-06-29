// Test basic Set functionality
// expect: success

// Basic Set creation
let mySet = new Set();

// Add operations
mySet.add("value1");
mySet.add(42);
mySet.add("value1"); // Duplicate should not increase size

// Test has
let has1 = mySet.has("value1");
let has2 = mySet.has(42);
let has3 = mySet.has("nonexistent");

// Test size
let size = mySet.size;

// Verify results
(has1 === true && has2 === true && has3 === false && size === 2) ? "success" : "failed"