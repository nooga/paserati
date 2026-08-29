// Regression test for nooga#90: WeakMap, WeakSet, WeakRef and
// FinalizationRegistry instances were falsey because Value.IsFalsey omitted
// their ValueTypes and the default arm reported unknown object types as falsey.
// Per ECMA-262 ToBoolean every object is truthy.
// expect: true

const checks = [
  !!new Map(),
  !!new Set(),
  !!new WeakMap(),
  !!new WeakSet(),
  !!new WeakRef({}),
  !!new FinalizationRegistry(() => {}),
];

// The guard idiom from the issue: a live WeakMap must not read as missing.
const wm = new WeakMap();
let cache: any = null;
cache = cache || wm;
const guardKeepsInstance = cache === wm;

checks.every((x) => x === true) && guardKeepsInstance;
