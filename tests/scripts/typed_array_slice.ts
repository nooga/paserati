// expect: 3
// Test TypedArray.prototype.slice() method

const arr = new Uint8Array([1, 2, 3, 4, 5]);
const sliced = arr.slice(1, 4);

// sliced should be a new array [2, 3, 4]
sliced.length;