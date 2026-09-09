// expect: all true

// Regression test for #377: `instanceof` was always false for ArrayBuffer and
// DataView values, and Object.getPrototypeOf reported Object.prototype for an
// ArrayBuffer. Typed arrays built by the runtime rather than by a constructor
// call (subarray/slice/from) hit the same hole via instanceof.

const ab = new ArrayBuffer(8);
const dv = new DataView(ab);
const u8 = new Uint8Array(4);

const checks: boolean[] = [
  // instanceof on the exotic buffer kinds
  ab instanceof ArrayBuffer,
  dv instanceof DataView,
  u8 instanceof Uint8Array,
  // a buffer reached through a typed array, i.e. one the runtime handed out
  // rather than one a `new ArrayBuffer()` call produced
  u8.buffer instanceof ArrayBuffer,
  // typed array views the runtime builds internally
  u8.subarray(0, 2) instanceof Uint8Array,
  u8.slice() instanceof Uint8Array,
  Uint8Array.from([1, 2]) instanceof Uint8Array,
  // everything inherits from Object
  ab instanceof Object,
  dv instanceof Object,
  u8 instanceof Object,
  // the [[Prototype]] accessors all agree with instanceof
  Object.getPrototypeOf(ab) === ArrayBuffer.prototype,
  Object.getPrototypeOf(dv) === DataView.prototype,
  Object.getPrototypeOf(u8.subarray(0, 2)) === Uint8Array.prototype,
  Reflect.getPrototypeOf(ab) === ArrayBuffer.prototype,
  Reflect.getPrototypeOf(dv) === DataView.prototype,
  ArrayBuffer.prototype.isPrototypeOf(ab),
  DataView.prototype.isPrototypeOf(dv),
  Object.prototype.isPrototypeOf(ab),
  // .constructor already pointed at the real global; keep it pinned
  ab.constructor === ArrayBuffer,
  dv.constructor === DataView,
  // kinds that already worked, guarding against a regression
  {} instanceof Object,
  [] instanceof Array,
  new Map() instanceof Map,
  (async function () {}) instanceof Function,
  function* () {} instanceof Function,
  // and instanceof must still say no when it should
  !(ab instanceof Uint8Array),
  !(dv instanceof ArrayBuffer),
  !(u8 instanceof DataView),
  !(ab instanceof DataView),
];

checks.every((c) => c) ? "all true" : "some false: " + checks.indexOf(false);
