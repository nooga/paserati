// Test that array destructuring uses Symbol.iterator
// expect: x:1 y:2 z:42, called:true
let called = false;
Array.prototype[Symbol.iterator] = function* () {
  called = true;
  if (this.length > 0) yield this[0];
  if (this.length > 1) yield this[1];
  if (this.length > 2) yield 42; // Override third element
};

let arr = [1, 2, 3];
let [x, y, z] = arr;
`x:${x} y:${y} z:${z}, called:${called}`;
