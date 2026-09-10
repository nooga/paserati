// expect: true

// #395: new Blob(...) must produce a real Blob instance - constructor,
// prototype chain, and instanceof must all agree, matching the DOM/File API
// spec (and real Node/undici, which relies on `x instanceof Blob` to detect
// Blob/File-shaped fetch bodies).
const b = new Blob(["hi"]);
const check1 = b instanceof Blob;
const check2 = Object.getPrototypeOf(b) === Blob.prototype;

// slice() must return an instance sharing the same prototype, not a bare
// Object (it was previously constructed with vmInstance.ObjectPrototype).
const sliced = b.slice(0, 1);
const check3 = sliced instanceof Blob;
const check4 = Object.getPrototypeOf(sliced) === Blob.prototype;

check1 && check2 && check3 && check4;
