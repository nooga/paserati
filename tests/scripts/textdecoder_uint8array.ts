// TextDecoder.decode() must decode a real Uint8Array/ArrayBuffer, not just
// a plain JS array - every fetch()/ReadableStream body chunk is one (#212).
const decoder = new TextDecoder();

const bytes = new Uint8Array([104, 101, 108, 108, 111]); // "hello"
const fromTypedArray = decoder.decode(bytes);

const fromArrayBuffer = decoder.decode(bytes.buffer);

// A view over a slice of a larger buffer should decode only its own window.
const buf = new Uint8Array([0, 0, 104, 105, 0, 0]).buffer;
const view = new Uint8Array(buf, 2, 2); // "hi"
const fromSlicedView = decoder.decode(view);

`${fromTypedArray}|${fromArrayBuffer}|${fromSlicedView}`;

// expect: hello|hello|hi
