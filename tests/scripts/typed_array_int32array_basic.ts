// expect: -2147483648
// Test Int32Array with negative values

const arr = new Int32Array(3);
arr[0] = 2147483647;  // max int32
arr[1] = -2147483648; // min int32
arr[2] = 2147483648;  // should wrap to -2147483648

arr[2];