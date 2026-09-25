// `__proto__: v` in an object literal accepts any object, including
// functions and arrays, not only plain objects.
// no-typecheck
// expect: true,true,true,true
function F() {}
const arr = [1];
[
  Object.getPrototypeOf({ __proto__: F }) === F,
  Object.getPrototypeOf({ __proto__: arr }) === arr,
  Object.getPrototypeOf({ __proto__: 5 }) === Object.prototype,
  Object.getPrototypeOf({ __proto__: null }) === null,
].join(",");
