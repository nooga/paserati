// expect: 255
// Test Uint8Array element access

const arr = new Uint8Array(5);
arr[0] = 255;
arr[1] = 256; // Should wrap to 0
arr[2] = -1;  // Should wrap to 255

arr[0]; // 255