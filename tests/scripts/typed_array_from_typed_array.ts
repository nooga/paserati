// expect: 7
// Regression test for #386: new TypedArray(otherTypedArray) must copy
// elements, not produce a zero-length array (same-type and cross-type).

const src = new Uint8Array([1, 2, 3]);
const sameType = new Uint8Array(src);
const crossType = new Int32Array(src);
const fromFloat = new Float64Array(new Float64Array([1.5, 2.5]));

let ok = 1;
if (sameType.length !== 3 || sameType[0] !== 1 || sameType[1] !== 2 || sameType[2] !== 3) ok = 0;
if (crossType.length !== 3 || crossType[0] !== 1 || crossType[1] !== 2 || crossType[2] !== 3) ok = 0;
if (fromFloat.length !== 2 || fromFloat[0] !== 1.5 || fromFloat[1] !== 2.5) ok = 0;

src.length + sameType.length + ok;
