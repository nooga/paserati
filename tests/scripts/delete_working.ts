// expect: true

// Test delete operator functionality
let obj = { x: 10, y: 20 };

// Delete existing property
let result1 = delete obj.x; // Should return true

// Check that property was actually deleted
let xUndefined = obj.x === undefined; // Should be true

// Check other property still exists
let yExists = obj.y === 20; // Should be true

// All operations should be true
result1 && xUndefined && yExists;