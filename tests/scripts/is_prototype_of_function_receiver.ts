// isPrototypeOf with a function receiver follows the function's real
// [[Prototype]]: built-in error hierarchy, class heritage, setPrototypeOf (#565).
// no-typecheck
// expect: true,true,true,true,true,false
class E2 extends Error {}
class E3 extends E2 {}
function F() {}
const g = Object.setPrototypeOf(function G() {}, F);
[
  Error.isPrototypeOf(RangeError),
  Error.isPrototypeOf(E3),
  E2.isPrototypeOf(E3),
  F.isPrototypeOf(g),
  Object.prototype.isPrototypeOf.call(Error, E2),
  RangeError.isPrototypeOf(Error),
].join(",");
