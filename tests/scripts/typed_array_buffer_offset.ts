// expect: 255
// Test creating typed array with offset into buffer

const buffer = new ArrayBuffer(16);
const fullView = new Uint8Array(buffer);
fullView[4] = 255;
fullView[5] = 128;

// Create view starting at byte offset 4, length 4
const offsetView = new Uint8Array(buffer, 4, 4);
offsetView[0]; // Should be 255