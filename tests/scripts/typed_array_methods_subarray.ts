// expect: 3
// Test TypedArray.prototype.subarray() method

const arr = new Uint8Array([0, 1, 2, 3, 4, 5]);
const sub = arr.subarray(2, 5);

// sub should be a view of [2, 3, 4]
sub.length;