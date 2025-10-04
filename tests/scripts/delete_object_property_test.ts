// Test delete operator on object properties
// expect: pass

const obj = { x: 1, y: 2, z: 3 };

// Delete existing property should return true
if (delete obj.x !== true) {
  throw new Error('Expected delete obj.x to return true');
}

// Property should no longer exist
if ('x' in obj) {
  throw new Error('Property x should be deleted');
}

// Delete non-existent property should return true (use bracket notation to avoid type error)
if (delete obj['nonexistent'] !== true) {
  throw new Error('Expected delete obj.nonexistent to return true');
}

// Other properties should still exist
if (obj.y !== 2 || obj.z !== 3) {
  throw new Error('Other properties should remain');
}

'pass';
