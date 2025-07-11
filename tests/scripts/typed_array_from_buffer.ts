// expect: 42
// Test creating typed array from ArrayBuffer

const buffer = new ArrayBuffer(16);
const view = new Uint8Array(buffer);
view[3] = 42;
view[3];