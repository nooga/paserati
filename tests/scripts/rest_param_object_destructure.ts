// Test rest parameter with object destructuring
// no-typecheck
// expect: 6

function f(...[{x, y, z}]) {
  return x + y + z;
}

f({x: 1, y: 2, z: 3});
