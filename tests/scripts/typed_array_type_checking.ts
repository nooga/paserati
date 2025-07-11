// expect: true
// Test TypeScript type checking for typed arrays

const buffer: ArrayBuffer = new ArrayBuffer(16);
const uint8View: Uint8Array = new Uint8Array(buffer);
const int32View: Int32Array = new Int32Array(buffer);

// Type checking should allow this
function processBytes(bytes: Uint8Array): number {
    return bytes.length;
}

processBytes(uint8View) === 16;