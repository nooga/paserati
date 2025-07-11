// expect: 4
// Test TypedArray.prototype.set() method

const arr1 = new Uint8Array([1, 2, 3]);
const arr2 = new Uint8Array(5);

// Copy arr1 into arr2 starting at index 1
arr2.set(arr1, 1);

// arr2 should be [0, 1, 2, 3, 0]
arr2[3] + arr2[1];