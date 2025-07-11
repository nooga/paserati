// expect: 4
// Test typed array byte properties

const arr = new Int32Array(5);

// Each Int32 element is 4 bytes
arr.BYTES_PER_ELEMENT;