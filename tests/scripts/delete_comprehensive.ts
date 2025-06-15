// expect: true

// Test delete operator functionality
let obj: { x: number; y: number; z?: number; nonexistent?: number } = {
  x: 10,
  y: 20,
};

// Delete existing property
let result1 = delete obj.x; // Should return true

// Delete non-existent property
let result2 = delete obj.nonexistent; // Should return true

// Check that property was actually deleted
let xUndefined = obj.x === undefined; // Should be true

// Check other properties still exist
let yExists = obj.y === 20; // Should be true

// Multiple operations should all be true
result1 && result2 && xUndefined && yExists;
