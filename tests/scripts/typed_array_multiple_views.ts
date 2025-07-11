// expect: 513
// Test multiple typed array views on same buffer

const buffer = new ArrayBuffer(8);
const uint8 = new Uint8Array(buffer);
const uint16 = new Uint16Array(buffer);

// Set bytes [1, 2, 0, 0, ...]
uint8[0] = 1;
uint8[1] = 2;

// uint16[0] should read bytes 0-1 as little-endian 16-bit int
// which is 0x0201 = 513
uint16[0];