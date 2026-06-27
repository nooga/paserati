// expect: true
// Subclassing native constructors must give instances the subclass prototype
// (ECMAScript: super() into a native ctor uses newTarget.prototype), so
// `new Sub() instanceof Sub` and `instanceof Base` both hold.

class SubBoolean extends Boolean {}
class SubNumber extends Number {}
class SubError extends Error {}
class SubArray extends Array {}
class SubInt8 extends Int8Array {}
class SubRegExp extends RegExp {}

const sb = new SubBoolean();
const sn = new SubNumber();
const se = new SubError();
const sa = new SubArray();
const si = new SubInt8();
const sr = new SubRegExp("a");

const checks: boolean[] = [
  sb instanceof SubBoolean, sb instanceof Boolean,
  sn instanceof SubNumber, sn instanceof Number,
  se instanceof SubError, se instanceof Error,
  sa instanceof SubArray, sa instanceof Array,
  si instanceof SubInt8, si instanceof Int8Array,
  sr instanceof SubRegExp, sr instanceof RegExp,
];

checks.every((x) => x === true);
